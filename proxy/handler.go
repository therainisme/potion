package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/therainisme/potion/util"
)

func HandleRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	util.LogDebug("Method: %s, URL: %s", r.Method, r.URL)

	switch path {
	case "":
		handleRootPath(w, r)
	case "sitemap.xml":
		handleSitemap(w, r)
	case "robots.txt":
		handleRobots(w, r)
	default:
		proxyRequest(w, r, path)
	}
}

// handleRootPath handles the root path and redirects to the blog
func handleRootPath(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host

	redirectURL := fmt.Sprintf("%s://%s/%s", scheme, host, util.GetSiteSlug())
	util.LogDebug("Redirecting to blog: %s", util.GetSiteSlug())
	http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
}

// proxyRequest proxies the request to the notion site
func proxyRequest(w http.ResponseWriter, r *http.Request, path string) {
	requestURL := buildRequestURL(r)
	req, err := createProxyRequest(r, requestURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := sendProxyRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	switch {
	case path == "api/v3/getPublicPageData":
		handlePublicPageData(w, resp)
	case strings.Contains(resp.Header.Get("Content-Type"), "text/html"):
		handleHTMLResponse(w, resp)
	default:
		handleDefaultResponse(w, resp)
	}
}

func buildRequestURL(r *http.Request) string {
	requestURL := fmt.Sprintf("%s%s", util.GetSiteDomain(), r.URL.Path)

	if strings.Contains(requestURL, "notion.site/image/https://") {
		requestURL = handleImageURL(requestURL)
	}

	if r.URL.RawQuery != "" {
		requestURL += "?" + r.URL.RawQuery
	}
	util.LogDebug("Proxying request to: %s", requestURL)
	return requestURL
}

// handleImageURL transforms the notion image URL
// The images url of notion is https://notion.site/image/https://...
// We need to escape the url to make it a valid url
// For example, https://notion.site/image/https://www.google.com/image.png
// will be transformed to https://notion.site/image/https%3A%2F%2Fwww.google.com%2Fimage.png
func handleImageURL(requestURL string) string {
	idx := strings.Index(requestURL[strings.Index(requestURL, "notion.site/image/")+len("notion.site/image/"):], "https://")
	if idx != -1 {
		baseURL := requestURL[:strings.Index(requestURL, "notion.site/image/")+len("notion.site/image/")]
		imageURL := requestURL[strings.Index(requestURL, "notion.site/image/")+len("notion.site/image/"):]
		return baseURL + url.QueryEscape(imageURL)
	}
	return requestURL
}

func createProxyRequest(r *http.Request, requestURL string) (*http.Request, error) {
	req, err := http.NewRequest(r.Method, requestURL, r.Body)
	if err != nil {
		return nil, err
	}

	// copy request headers
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	return req, nil
}

func sendProxyRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client.Do(req)
}

// handlePublicPageData fix "Continue to external site" error.
// The response is a json object, we need to remove the "requireInterstitial" field
func handlePublicPageData(w http.ResponseWriter, resp *http.Response) {
	reader := getReader(resp)
	body, err := io.ReadAll(reader)
	if err != nil {
		util.LogError("Failed to read response body: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		util.LogError("Failed to parse JSON: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	delete(data, "requireInterstitial")

	modifiedJSON, err := json.Marshal(data)
	if err != nil {
		util.LogError("Failed to marshal JSON: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if resp.Header.Get("Content-Encoding") == "gzip" {
		sendGzippedResponse(w, resp, string(modifiedJSON))
	} else {
		w.WriteHeader(resp.StatusCode)
		w.Write(modifiedJSON)
	}
}

// handleHTMLResponse handles the HTML response
// It injects the script to set the page title
// If the response is gzip compressed, it will send the gzipped response
// Otherwise, it will send the uncompressed response
func handleHTMLResponse(w http.ResponseWriter, resp *http.Response) {
	reader := getReader(resp)
	body, err := io.ReadAll(reader)
	if err != nil {
		util.LogError("Failed to read HTML response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	htmlString := injectScript(string(body))

	if resp.Header.Get("Content-Encoding") == "gzip" {
		sendGzippedResponse(w, resp, htmlString)
	} else {
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(htmlString))
	}
}

func injectScript(htmlString string) string {
	script := fmt.Sprintf(`
	<meta name="google-site-verification" content="%s" />
	<style>
		/* Hide the "More actions" button and the button after it */
		.notion-topbar [role="button"][tabindex="0"][aria-label],
		.notion-topbar [role="button"][tabindex="0"][style*="border: 1px solid"] {
			display: none !important;
		}
		/* Hide the mobile version of the buttons if they exist */
		.notion-topbar-mobile [role="button"][tabindex="0"][aria-label],
		.notion-topbar-mobile [role="button"][tabindex="0"][style*="border: 1px solid"] {
			display: none !important;
		}
		/* Hide "Free Notion" button based on background color */
		.notion-topbar [role="button"][style*="background: var(--c-bacAccPri)"],
		.notion-topbar-mobile [role="button"][style*="background: var(--c-bacAccPri)"] {
			display: none !important;
		}
	</style>
	<script>
		// Set the page title and description
		const PAGE_TITLE = "%s";
		const PAGE_DESCRIPTION = "%s";

		// Create a MutationObserver to listen to DOM changes
		const observer = new MutationObserver(() => {
			// Update page info
			const titleElement = document.querySelector("title");
			const metaDescriptions = [
				document.querySelector('meta[name="description"]'),
				document.querySelector('meta[property="og:description"]')
			];

			// Update or create title
			if (titleElement && titleElement.textContent !== PAGE_TITLE) {
				titleElement.textContent = PAGE_TITLE;
			}

			// Update existing meta descriptions
			metaDescriptions.forEach(meta => {
				if (meta && meta.getAttribute("content") !== PAGE_DESCRIPTION) {
					meta.setAttribute("content", PAGE_DESCRIPTION);
				}
			});

			// Create meta descriptions if they don't exist
			if (!document.querySelector('meta[name="description"]') && document.head) {
				const meta = document.createElement("meta");
				meta.setAttribute("name", "description");
				meta.setAttribute("content", PAGE_DESCRIPTION);
				document.head.appendChild(meta);
			}
			if (!document.querySelector('meta[property="og:description"]') && document.head) {
				const meta = document.createElement("meta");
				meta.setAttribute("property", "og:description");
				meta.setAttribute("content", PAGE_DESCRIPTION);
				document.head.appendChild(meta);
			}
		});

		// Start observing head changes
		observer.observe(document.head, {
			childList: true,
			subtree: true,
			characterData: true
		});

		// Also observe body for any dynamically added title elements
		observer.observe(document.body, {
			childList: true,
			subtree: true,
			characterData: true
		});
	</script>`, util.GetGoogleSiteVerification(), util.GetPageTitle(), util.GetPageDescription())

	return strings.Replace(htmlString, "</head>", script+"</head>", 1)
}

// sendGzippedResponse sends the gzipped response
func sendGzippedResponse(w http.ResponseWriter, resp *http.Response, content string) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(content)); err != nil {
		util.LogError("Failed to gzip response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := gz.Close(); err != nil {
		util.LogError("Failed to close gzip writer: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(resp.StatusCode)
	w.Write(buf.Bytes())
}

func handleDefaultResponse(w http.ResponseWriter, resp *http.Response) {
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// getReader returns the reader of the response
// If the response is gzip compressed, it will return the gzip reader
// Otherwise, it will return the original response body
func getReader(resp *http.Response) io.ReadCloser {
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			util.LogError("Failed to create gzip reader: %v", err)
			return resp.Body
		}
		return reader
	}
	return resp.Body
}

// handleRobots handles the robots.txt request
func handleRobots(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host

	content := fmt.Sprintf(`User-agent: *
Allow: /

Sitemap: %s://%s/sitemap.xml`, scheme, host)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}
