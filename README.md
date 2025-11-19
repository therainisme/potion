# Potion

A lightweight proxy tool for customizing Notion pages, making your Notion blog more SEO-friendly and customizable. With Potion, you can:

1. Access Notion pages through your own server, with support for custom domains
2. Customize page title and description
3. Support sitemap.xml

## Quick Start

1. Clone the repository:

```bash
git clone https://github.com/therainisme/potion.git
cd potion
```

2. Create and edit `.env` file:

```env
PORT=8080
SITE_DOMAIN=https://your-blog.notion.site
SITE_SLUG=your-page-slug
PAGE_TITLE=Your custom title
SITEMAP_ID=Your blog database id
PAGE_DESCRIPTION=Your custom description
GOOGLE_SITE_VERIFICATION=Your Google Search Console verification code
```

3. Run:

```bash
go run .
```

## Docker Deployment

### Using Docker Compose

```yaml
services:
  potion:
    image: therainisme/potion
    container_name: potion
    environment:
      - PORT=8080
      - SITE_DOMAIN=https://your-blog.notion.site
      - SITE_SLUG=your-page-slug
      - PAGE_TITLE=Your custom title
      - SITEMAP_ID=Your blog database id
      - PAGE_DESCRIPTION=Your custom description
      - GOOGLE_SITE_VERIFICATION=Your Google Search Console verification code
    # Or use .env file
    # volumes:
    #   - .env:/app/.env
    ports:
      - "8080:8080"
    restart: always
```

### Using Docker CLI

```bash
# Pull the image
docker pull therainisme/potion

# Run with environment variables
docker run -d \
  --name potion \
  -p 8080:8080 \
  -e SITE_DOMAIN=https://your-blog.notion.site \
  -e SITE_SLUG=your-page-slug \
  -e PAGE_TITLE="Your custom title" \
  -e SITEMAP_ID="Your blog database id" \
  -e PAGE_DESCRIPTION="Your custom description" \
  -e GOOGLE_SITE_VERIFICATION="Your Google Search Console verification code" \
  therainisme/potion

# Or run with .env file
docker run -d \
  --name potion \
  -p 8080:8080 \
  -v $(pwd)/.env:/app/.env \
  therainisme/potion
```

## Example Site

Visit my blog to see Potion in action: https://blog.therainisme.com

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| PORT | Server port | 8080 |
| SITE_DOMAIN | Your Notion site domain | https://your-blog.notion.site |
| SITE_SLUG | Page slug | your-page-slug |
| PAGE_TITLE | Custom page title | My Blog |
| SITEMAP_ID (optional) | Your blog database id | xxxxxxxxx |
| PAGE_DESCRIPTION | Custom page description | Welcome to my blog |
| GOOGLE_SITE_VERIFICATION (optional) | Your Google Search Console verification code | xxxxxxxxxxxxx |