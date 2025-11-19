package proxy

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/therainisme/potion/util"
)

type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	Xmlns   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

type URL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

func loadDatabasePages(r *http.Request) ([]URL, error) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// 1. First request to get collection_id and view_id
	payload := map[string]interface{}{
		"page": map[string]interface{}{
			"id": util.GetSitemapID(),
		},
		"limit":           30,
		"cursor":          map[string]interface{}{"stack": []interface{}{}},
		"verticalColumns": false,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/api/v3/loadCachedPageChunkV2", util.GetSiteDomain()),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	var viewID, collectionID string
	if recordMap, ok := result["recordMap"].(map[string]interface{}); ok {
		if block, ok := recordMap["block"].(map[string]interface{}); ok {
			if blockValue, ok := block[util.GetSitemapID()].(map[string]interface{}); ok {
				if value, ok := blockValue["value"].(map[string]interface{}); ok {
					if viewIds, ok := value["view_ids"].([]interface{}); ok && len(viewIds) > 0 {
						viewID = viewIds[0].(string)
					}
					if colID, ok := value["collection_id"].(string); ok {
						collectionID = colID
					}
				}
			}
		}
	}

	if viewID == "" || collectionID == "" {
		return nil, fmt.Errorf("failed to find viewID or collectionID")
	}

	// 2. Second request to queryCollection
	queryPayload := map[string]interface{}{
		"collection": map[string]interface{}{
			"id": collectionID,
		},
		"collectionView": map[string]interface{}{
			"id": viewID,
		},
		"loader": map[string]interface{}{
			"type": "reducer",
			"reducers": map[string]interface{}{
				"collection_group_results": map[string]interface{}{
					"type":  "results",
					"limit": 50,
				},
			},
			"sort":         []interface{}{},
			"searchQuery":  "",
			"userTimeZone": "Asia/Shanghai",
		},
	}

	queryJsonData, err := json.Marshal(queryPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query JSON: %v", err)
	}

	queryReq, err := http.NewRequest("POST",
		fmt.Sprintf("%s/api/v3/queryCollection", util.GetSiteDomain()),
		bytes.NewBuffer(queryJsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create query request: %v", err)
	}

	queryReq.Header.Set("Content-Type", "application/json")
	queryReq.Header.Set("Accept", "*/*")
	queryReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	queryResp, err := client.Do(queryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send query request: %v", err)
	}
	defer queryResp.Body.Close()

	queryBody, err := io.ReadAll(queryResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read query response: %v", err)
	}

	var queryResult map[string]interface{}
	if err := json.Unmarshal(queryBody, &queryResult); err != nil {
		return nil, fmt.Errorf("failed to parse query JSON: %v", err)
	}

	var urls []URL
	if resultData, ok := queryResult["result"].(map[string]interface{}); ok {
		if reducerResults, ok := resultData["reducerResults"].(map[string]interface{}); ok {
			if collectionGroupResults, ok := reducerResults["collection_group_results"].(map[string]interface{}); ok {
				if blockIds, ok := collectionGroupResults["blockIds"].([]interface{}); ok {
					for _, id := range blockIds {
						if pageId, ok := id.(string); ok {
							urls = append(urls, URL{
								Loc:        fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(pageId, "-", "")),
								LastMod:    time.Now().Format("2006-01-02"),
								ChangeFreq: "daily",
								Priority:   0.8,
							})
						}
					}
				}
			}
		}
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no pages found in the collection view")
	}

	return urls, nil
}

func handleSitemap(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Create base URL set
	urlset := URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs: []URL{
			{
				Loc:        fmt.Sprintf("%s/%s", baseURL, util.GetSiteSlug()),
				LastMod:    time.Now().Format("2006-01-02"),
				ChangeFreq: "daily",
				Priority:   1.0,
			},
		},
	}

	// Load database pages
	if dbUrls, err := loadDatabasePages(r); err == nil {
		urlset.URLs = append(urlset.URLs, dbUrls...)
	} else {
		util.LogError("Failed to load database pages: %v", err)
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml.Header))

	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(urlset); err != nil {
		util.LogError("Failed to encode sitemap: %v", err)
		http.Error(w, "Failed to generate sitemap", http.StatusInternalServerError)
		return
	}
}
