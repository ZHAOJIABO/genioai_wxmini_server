package utils

import (
	"testing"
)

func TestCleanURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with query params",
			input:    "https://example.com/image.jpg?size=1080x525",
			expected: "https://example.com/image.jpg",
		},
		{
			name:     "URL with multiple query params",
			input:    "https://example.com/image.jpg?size=1080x525&quality=high&token=abc123",
			expected: "https://example.com/image.jpg",
		},
		{
			name:     "URL without query params",
			input:    "https://example.com/image.jpg",
			expected: "https://example.com/image.jpg",
		},
		{
			name:     "Non-HTTP URL",
			input:    "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD",
			expected: "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanURL(tt.input)
			if result != tt.expected {
				t.Errorf("CleanURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
