package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/proxera/backend/internal/database"
)

// GeoResult holds geolocation info for a single IP
type GeoResult struct {
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	City        string `json:"city"`
	Region      string `json:"region"`
}

// ipAPIResponse is the response format from ip-api.com batch endpoint
type ipAPIResponse struct {
	Status      string  `json:"status"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"regionName"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	ISP         string  `json:"isp"`
	Query       string  `json:"query"`
}

// LookupGeo looks up geolocation for a list of IPs, using cache first
func LookupGeo(ips []string) (map[string]GeoResult, error) {
	if len(ips) == 0 {
		return map[string]GeoResult{}, nil
	}

	results := make(map[string]GeoResult, len(ips))

	// 1. Check cache (30-day TTL)
	rows, err := database.DB.Query(context.Background(),
		`SELECT ip_address, country, country_code, city, region
		 FROM geo_cache WHERE ip_address = ANY($1::text[])
		 AND looked_up_at > NOW() - INTERVAL '30 days'`, ips)
	if err != nil {
		slog.Error("cache query error", "component", "geo", "error", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var ip, country, cc, city, region string
			if err := rows.Scan(&ip, &country, &cc, &city, &region); err == nil {
				results[ip] = GeoResult{
					Country:     country,
					CountryCode: cc,
					City:        city,
					Region:      region,
				}
			}
		}
	}

	// 2. Find cache misses
	var misses []string
	for _, ip := range ips {
		if _, ok := results[ip]; !ok {
			misses = append(misses, ip)
		}
	}

	if len(misses) == 0 {
		return results, nil
	}

	// 3. Batch lookup via ip-api.com (max 100 per request)
	client := &http.Client{Timeout: HTTPClientTimeout}

	for i := 0; i < len(misses); i += 100 {
		end := i + 100
		if end > len(misses) {
			end = len(misses)
		}
		batch := misses[i:end]

		body, err := json.Marshal(batch)
		if err != nil {
			continue
		}

		req, err := http.NewRequest("POST", "http://ip-api.com/batch?fields=status,country,countryCode,regionName,city,lat,lon,isp,query", bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			slog.Error("batch lookup error", "component", "geo", "error", err)
			continue
		}

		var apiResults []ipAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResults); err != nil {
			_ = resp.Body.Close()
			slog.Error("failed to decode batch response", "component", "geo", "error", err)
			continue
		}
		_ = resp.Body.Close()

		// Upsert into cache and populate results
		for _, r := range apiResults {
			if r.Status != "success" {
				continue
			}

			results[r.Query] = GeoResult{
				Country:     r.Country,
				CountryCode: r.CountryCode,
				City:        r.City,
				Region:      r.Region,
			}

			// Upsert cache
			_, err := database.DB.Exec(context.Background(),
				`INSERT INTO geo_cache (ip_address, country, country_code, city, region, lat, lon, isp, looked_up_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
				 ON CONFLICT (ip_address) DO UPDATE SET
					country = EXCLUDED.country,
					country_code = EXCLUDED.country_code,
					city = EXCLUDED.city,
					region = EXCLUDED.region,
					lat = EXCLUDED.lat,
					lon = EXCLUDED.lon,
					isp = EXCLUDED.isp,
					looked_up_at = NOW()`,
				r.Query, r.Country, r.CountryCode, r.City, r.Region, r.Lat, r.Lon, r.ISP,
			)
			if err != nil {
				slog.Error("failed to cache geo", "component", "geo", "ip", r.Query, "error", err)
			}
		}

		// Rate limit: ~15 req/min
		if end < len(misses) {
			time.Sleep(4 * time.Second)
		}
	}

	// For any IPs that still have no result, fill with empty
	for _, ip := range ips {
		if _, ok := results[ip]; !ok {
			results[ip] = GeoResult{}
		}
	}

	return results, nil
}
