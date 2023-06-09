package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadDrivers(t *testing.T) {
	// arrange
	// act
	actual := loadDrivers()
	// assert
	if actual == nil {
		t.Error("Expected drivers but got nil")
	}
}

func TestListDrivers(t *testing.T) {
	req, err := http.NewRequest("GET", "/drivers", nil)

	if err != nil {
		t.Errorf("Error creating a new request: %v", err)
	}

	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(ListDrivers)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code. Expected: %d. Got: %d.", http.StatusOK, status)
	}
}

func TestGetDriver(t *testing.T) {
	requestURL := fmt.Sprintf("/drivers/%s", "45688cd6-7a27-4a7b-89c5-a9b604eefe2f")

	req, err := http.NewRequest("GET", requestURL, nil)

	if err != nil {
		t.Errorf("Error creating a new request: %v", err)
	}

	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(GetDriver)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code. Expected: %d. Got: %d.", http.StatusOK, status)
	}
}
