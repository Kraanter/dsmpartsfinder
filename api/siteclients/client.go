package siteclients

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"
)

// Part represents a car part from any site client
type Part struct {
	ID           string    `json:"id"`
	Description  string    `json:"description"`
	TypeName     string    `json:"type_name"`
	Name         string    `json:"name"`
	ImageBase64  string    `json:"image_base64"`
	URL          string    `json:"url"`
	SiteID       int       `json:"site_id"`
	Price        string    `json:"price"`
	CreationDate time.Time `json:"creation_date"`
}

// SearchParams represents the search parameters for finding parts
type SearchParams struct {
	VehicleType string
	Make        string
	BaseModel   string
	Model       string
	YearFrom    int
	YearTo      int
	Offset      int
	Limit       int
}

// SiteClient defines the interface that all site clients must implement
type SiteClient interface {
	// GetName returns the name of the site client
	GetName() string

	// FetchParts fetches parts from the site based on search parameters
	FetchParts(ctx context.Context, params SearchParams) ([]Part, error)

	// GetSiteID returns the database ID of the site this client represents
	GetSiteID() int
}

func CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}
