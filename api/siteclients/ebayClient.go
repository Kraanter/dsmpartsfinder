package siteclients

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Constants for eBay URLs
const (
	// Sandbox URLs
	sandboxTokenURL  = "https://api.sandbox.ebay.com/identity/v1/oauth2/token"
	sandboxSearchURL = "https://api.sandbox.ebay.com/buy/browse/v1/item_summary/search"

	// Production URLs
	prodTokenURL  = "https://api.ebay.com/identity/v1/oauth2/token"
	prodSearchURL = "https://api.ebay.com/buy/browse/v1/item_summary/search"
)

// TokenResponse represents the OAuth token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type EbayBrowseResponse struct {
	Total         int        `json:"total"`
	ItemSummaries []EbayItem `json:"itemSummaries"`
	Limit         int        `json:"limit"`
	Offset        int        `json:"offset"`
}

type EbayItem struct {
	ItemID string `json:"itemId"`
	Title  string `json:"title"`
	Price  struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"price"`
	Condition       string    `json:"condition"`
	ItemWebURL      string    `json:"itemWebUrl"`
	ItemOriginDate  time.Time `json:"itemOriginDate"`
	ThumbnailImages []struct {
		ImageURL string `json:"imageUrl"`
	} `json:"thumbnailImages"`
}

// Item represents a single search result item
type Item struct {
	ItemID string `json:"itemId"`
	Title  string `json:"title"`
	Price  struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"price"`
	Condition    string `json:"condition"`
	ItemWebURL   string `json:"itemWebUrl"`
	ThumbnailURL string `json:"thumbnailImages,omitempty"`
}

// EbayClient implements the SiteClient interface for eBay
type EbayClient struct {
	baseURL      string
	httpClient   *http.Client
	siteID       int
	accessToken  string // token used for authentication
	clientID     string // eBay App ID (Client ID)
	clientSecret string // eBay Client Secret
	isSandbox    bool   // Indicates if the client is in sandbox mode
}

type ebayAPIError struct {
	Errors []struct {
		ErrorID      int64    `json:"errorId"`
		Domain       string   `json:"domain"`
		SubDomain    string   `json:"subDomain"`
		Category     string   `json:"category"`
		Message      string   `json:"message"`
		LongMessage  string   `json:"longMessage"`
		InputRefIds  []string `json:"inputRefIds"`
		OutputRefIds []string `json:"outputRefIds"`
		Parameters   []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"parameters"`
	} `json:"errors"`
}

// NewEbayClient creates a new EbayClient
func NewEbayClient(siteID int, appID string, clientSecret string, isSandbox bool) *EbayClient {
	return &EbayClient{
		baseURL:      "https://svcs.ebay.com/services/search/FindingService/v1",
		httpClient:   CreateHTTPClient(),
		siteID:       siteID,
		clientID:     appID,
		clientSecret: clientSecret,
		isSandbox:    isSandbox,
	}
}

// getTokenURL returns the appropriate token URL
func (c *EbayClient) getTokenURL() string {
	if c.isSandbox {
		return sandboxTokenURL
	}
	return prodTokenURL
}

// GetAccessToken retrieves an OAuth 2.0 access token
func (c *EbayClient) GetAccessToken() error {
	// Create Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("scope", "https://api.ebay.com/oauth/api_scope")

	// Create request
	req, err := http.NewRequest("POST", c.getTokenURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+auth)

	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken

	return nil
}

func (c *EbayClient) GetName() string {
	return "eBay"
}

func (c *EbayClient) GetSiteID() int {
	return c.siteID
}

// FetchParts fetches parts from eBay based on search parameters
func (c *EbayClient) FetchParts(ctx context.Context, params SearchParams) ([]Part, error) {
	log.Println("Fetching parts from eBay")
	c.GetAccessToken()
	log.Println("Access token retrieved")

	allParts := []Part{}
	offset := 0
	for {
		// Build query parameters
		query := url.Values{}
		query.Set("sort", "newlyListed")
		query.Set("limit", "200")
		query.Set("offset", fmt.Sprintf("%d", offset))
		query.Set("q", "(Mitsubishi Eclipse 2g, D32A)")
		query.Set("category_ids", "6030")

		apiURL := fmt.Sprintf("https://api.ebay.com/buy/browse/v1/item_summary/search?%s", query.Encode())
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add the access token to the request header
		req.Header.Set("Authorization", "Bearer "+c.accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Try to extract error message from body
			body, _ := io.ReadAll(resp.Body)
			var apiErr ebayAPIError
			msg := fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
			if len(body) > 0 {
				if err := json.Unmarshal(body, &apiErr); err == nil && len(apiErr.Errors) > 0 {
					msg += ": " + apiErr.Errors[0].Message
					if apiErr.Errors[0].LongMessage != "" {
						msg += " (" + apiErr.Errors[0].LongMessage + ")"
					}
				} else {
					msg += ": " + string(body)
				}
			}
			return nil, fmt.Errorf(msg)
		}

		var apiResponse EbayBrowseResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Convert eBay items to Part structs
		parts := []Part{}
		for _, item := range apiResponse.ItemSummaries {
			part := Part{
				ID:          item.ItemID,
				Description: item.Title,
				// TypeName:    "", // eBay doesn't provide type name directly
				Name:         item.Title,
				URL:          item.ItemWebURL,
				SiteID:       c.siteID,
				Price:        "€ " + item.Price.Value,
				CreationDate: item.ItemOriginDate,
			}
			// Fetch and convert image to base64
			if len(item.ThumbnailImages) > 0 && item.ThumbnailImages[0].ImageURL != "" {
				imageBase64, fetchErr := c.fetchImageAsBase64(ctx, item.ThumbnailImages[0].ImageURL)
				if fetchErr == nil {
					part.ImageBase64 = imageBase64
				}
			}
			parts = append(parts, part)
		}
		allParts = append(allParts, parts...)

		// If less than 200 results returned, we're done
		if len(parts) < 200 {
			break
		}
		offset += 200
	}
	return allParts, nil
}

// fetchImageAsBase64 fetches an image from a URL and returns it as a base64 string
func (c *EbayClient) fetchImageAsBase64(ctx context.Context, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create image request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code for image: %d", resp.StatusCode)
	}
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}
	return base64.StdEncoding.EncodeToString(imageData), nil
}
