package siteclients

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// parseEnterDate converts the enterDate string to a *time.Time
func parseEnterDate(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}

	// Try common date formats
	layouts := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		"02-01-2006",
		"02/01/2006",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return &t
		}
	}

	return nil
}

// SchadeAutosClient implements the SiteClient interface for schadeautos.nl
type SchadeAutosClient struct {
	baseURL    string
	httpClient *http.Client
	siteID     int
}

// NewSchadeAutosClient creates a new SchadeAutos client
func NewSchadeAutosClient(siteID int) *SchadeAutosClient {
	return &SchadeAutosClient{
		baseURL:    "https://www.schadeautos.nl",
		httpClient: CreateHTTPClient(),
		siteID:     siteID,
	}
}

// GetName returns the name of the site client
func (c *SchadeAutosClient) GetName() string {
	return "SchadeAutos"
}

// GetSiteID returns the database ID of the site
func (c *SchadeAutosClient) GetSiteID() int {
	return c.siteID
}

// schadeAutosResponse represents the JSON response structure from the API
type schadeAutosResponse struct {
	Result struct {
		Limited    bool                 `json:"limited"`
		Descr      string               `json:"descr"`
		Time       float64              `json:"time"`
		StockParts map[string]stockPart `json:"stockParts"`
	} `json:"result"`
}

// stockPart represents an individual part in the response
type stockPart struct {
	Prefix string `json:"prefix"`
	Code   string `json:"code"`
	Descr  string `json:"descr"`
	// TypeName      string        `json:"typeName"`
	EnterDate     string        `json:"enterDate"`
	Name          string        `json:"name"`
	Picture       string        `json:"picture"`
	MakeName      string        `json:"makeName"`
	BaseModelName string        `json:"baseModelName"`
	ModelName     string        `json:"modelName"`
	EngineName    string        `json:"engineName"`
	Year          interface{}   `json:"year"`
	PriceExcl     float64       `json:"priceExcl"`
	Price         string        `json:"price"`
	Nos           []interface{} `json:"nos"`
}

// FetchParts fetches parts from SchadeAutos based on search parameters
func (c *SchadeAutosClient) FetchParts(ctx context.Context, params SearchParams) ([]Part, error) {
	// Build form data
	formData := url.Values{}

	formData.Set("widget[vehicleType]", "P")
	formData.Set("widget[make]", "A0001E2D")
	formData.Set("widget[baseModel]", "A0001FHK")
	formData.Set("widget[model]", "A0001FHL")
	formData.Set("widget[type]", "")
	formData.Set("widget[vehicle]", "")

	// Set year from with default
	yearFrom := params.YearFrom
	if yearFrom == 0 {
		yearFrom = 1995
	}
	formData.Set("widget[yearFrom]", fmt.Sprintf("%d", yearFrom))

	// Set year to with default
	yearTo := params.YearTo
	if yearTo == 0 {
		yearTo = 2000
	}
	formData.Set("widget[yearTo]", fmt.Sprintf("%d", yearTo))

	formData.Set("widget[category]", "")
	formData.Set("widget[part]", "")
	formData.Set("widget[priceMax]", "")
	formData.Set("widget[query]", "")

	// Set offset with default
	offset := params.Offset
	if offset == 0 {
		offset = 0
	}
	formData.Set("offset", fmt.Sprintf("%d", offset))

	// Set limit with default
	limit := params.Limit
	if limit == 0 {
		limit = 3000
	}
	formData.Set("limit", fmt.Sprintf("%d", limit))

	formData.Set("order", "")
	formData.Set("shop", "")
	formData.Set("screenWidth", "2560")
	formData.Set("action", "search")

	// Create request
	apiURL := fmt.Sprintf("%s/parts/eng/search.json", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", fmt.Sprintf("%s/parts/eng/car-parts", c.baseURL))

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse response
	var apiResponse schadeAutosResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Log response info
	fmt.Printf("Response parsed. Limited: %v, Descr: %s, Parts count: %d\n",
		apiResponse.Result.Limited, apiResponse.Result.Descr, len(apiResponse.Result.StockParts))

	// Convert stock parts to Part structs
	parts := make([]Part, 0, len(apiResponse.Result.StockParts))
	for partID, stockPart := range apiResponse.Result.StockParts {
		part := Part{
			ID:          partID,
			Description: stockPart.Descr,
			// TypeName:    stockPart.TypeName,
			Name:         stockPart.Name,
			URL:          c.buildPartURL(partID, &stockPart),
			SiteID:       c.siteID,
			Price:        "€ " + stockPart.Price,
			CreationDate: *parseEnterDate(stockPart.EnterDate),
		}

		// Fetch and convert image to base64
		if stockPart.Picture != "" {
			imageBase64, fetchErr := c.fetchImageAsBase64(ctx, stockPart.Picture)
			if fetchErr != nil {
				// Log error but continue with other parts
				fmt.Printf("Warning: failed to fetch image for part %s: %v\n", partID, fetchErr)
			} else {
				part.ImageBase64 = imageBase64
			}
		}

		parts = append(parts, part)
	}

	return parts, nil
}

// buildPartURL constructs the URL for a specific part
func (c *SchadeAutosClient) buildPartURL(partID string, part *stockPart) string {
	// Build a URL like: https://www.schadeautos.nl/parts/eng/part/{partID}
	return fmt.Sprintf("%s/parts/eng/part/%s", c.baseURL, partID)
}

// fetchImageAsBase64 fetches an image from a URL and returns it as a base64 string
func (c *SchadeAutosClient) fetchImageAsBase64(ctx context.Context, imageURL string) (string, error) {
	// Handle relative URLs
	if strings.HasPrefix(imageURL, "//") {
		imageURL = "https:" + imageURL
	} else if strings.HasPrefix(imageURL, "/") {
		imageURL = c.baseURL + imageURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create image request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code for image: %d", resp.StatusCode)
	}

	// Read image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Convert to base64
	base64String := base64.StdEncoding.EncodeToString(imageData)

	return base64String, nil
}
