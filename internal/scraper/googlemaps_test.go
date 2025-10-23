package scraper

import (
	"testing"

	"assisted-venue-approval/internal/models"
)

func TestFillMissingVenueData_CoordinateOverride(t *testing.T) {
	// Create a venue with user-provided coordinates
	userLat := 40.7128
	userLng := -74.0060
	venue := models.Venue{
		ID:       1,
		Name:     "Test Venue",
		Location: "New York",
		Lat:      &userLat,
		Lng:      &userLng,
	}

	// Create Google data with different coordinates
	googleLat := 40.7580
	googleLng := -73.9855
	googleData := models.GooglePlaceData{
		PlaceID:          "test-place-id",
		Name:             "Test Venue Google",
		FormattedAddress: "123 Main St, New York, NY",
		Geometry: models.GoogleGeometry{
			Location: models.GoogleLatLng{
				Lat: googleLat,
				Lng: googleLng,
			},
		},
	}

	// Call fillMissingVenueData
	fillMissingVenueData(&venue, googleData)

	// Verify coordinates were overridden with Google values
	if venue.Lat == nil || venue.Lng == nil {
		t.Fatalf("coordinates are nil after fillMissingVenueData")
	}

	if *venue.Lat != googleLat {
		t.Errorf("expected Lat=%f, got %f", googleLat, *venue.Lat)
	}

	if *venue.Lng != googleLng {
		t.Errorf("expected Lng=%f, got %f", googleLng, *venue.Lng)
	}
}

func TestFillMissingVenueData_NoUserCoordinates(t *testing.T) {
	// Create a venue with no user-provided coordinates
	venue := models.Venue{
		ID:       2,
		Name:     "Test Venue 2",
		Location: "San Francisco",
	}

	// Create Google data
	googleLat := 37.7749
	googleLng := -122.4194
	googleData := models.GooglePlaceData{
		PlaceID:          "test-place-id-2",
		Name:             "Test Venue 2 Google",
		FormattedAddress: "456 Market St, San Francisco, CA",
		Geometry: models.GoogleGeometry{
			Location: models.GoogleLatLng{
				Lat: googleLat,
				Lng: googleLng,
			},
		},
	}

	// Call fillMissingVenueData
	fillMissingVenueData(&venue, googleData)

	// Verify coordinates were populated from Google
	if venue.Lat == nil || venue.Lng == nil {
		t.Fatalf("coordinates are nil after fillMissingVenueData")
	}

	if *venue.Lat != googleLat {
		t.Errorf("expected Lat=%f, got %f", googleLat, *venue.Lat)
	}

	if *venue.Lng != googleLng {
		t.Errorf("expected Lng=%f, got %f", googleLng, *venue.Lng)
	}
}

func TestFillMissingVenueData_ZeroUserCoordinates(t *testing.T) {
	// Create a venue with zero coordinates (invalid)
	userLat := 0.0
	userLng := 0.0
	venue := models.Venue{
		ID:       3,
		Name:     "Test Venue 3",
		Location: "Los Angeles",
		Lat:      &userLat,
		Lng:      &userLng,
	}

	// Create Google data
	googleLat := 34.0522
	googleLng := -118.2437
	googleData := models.GooglePlaceData{
		PlaceID:          "test-place-id-3",
		Name:             "Test Venue 3 Google",
		FormattedAddress: "789 Hollywood Blvd, Los Angeles, CA",
		Geometry: models.GoogleGeometry{
			Location: models.GoogleLatLng{
				Lat: googleLat,
				Lng: googleLng,
			},
		},
	}

	// Call fillMissingVenueData
	fillMissingVenueData(&venue, googleData)

	// Verify coordinates were overridden (even though they were zero)
	if venue.Lat == nil || venue.Lng == nil {
		t.Fatalf("coordinates are nil after fillMissingVenueData")
	}

	if *venue.Lat != googleLat {
		t.Errorf("expected Lat=%f, got %f", googleLat, *venue.Lat)
	}

	if *venue.Lng != googleLng {
		t.Errorf("expected Lng=%f, got %f", googleLng, *venue.Lng)
	}
}

func TestFillMissingVenueData_PreservesOtherFields(t *testing.T) {
	// Create a venue with other missing fields
	venue := models.Venue{
		ID:   4,
		Name: "Test Venue 4",
	}

	phone := "555-1234"
	website := "https://example.com"
	googleData := models.GooglePlaceData{
		PlaceID:          "test-place-id-4",
		Name:             "Test Venue 4 Google",
		FormattedAddress: "999 Test St",
		FormattedPhone:   phone,
		Website:          website,
		Geometry: models.GoogleGeometry{
			Location: models.GoogleLatLng{
				Lat: 35.0,
				Lng: -120.0,
			},
		},
	}

	// Call fillMissingVenueData
	fillMissingVenueData(&venue, googleData)

	// Verify phone was filled
	if venue.Phone == nil || *venue.Phone != phone {
		t.Errorf("expected Phone=%s, got %v", phone, venue.Phone)
	}

	// Verify website was filled
	if venue.URL == nil || *venue.URL != website {
		t.Errorf("expected URL=%s, got %v", website, venue.URL)
	}

	// Verify coordinates were set
	if venue.Lat == nil || *venue.Lat != 35.0 {
		t.Errorf("expected Lat=35.0, got %v", venue.Lat)
	}
}
