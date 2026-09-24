package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyFixedImageInputsOverridesClientValue(t *testing.T) {
	params := map[string]string{
		"LoadImage1": "https://example.com/client.jpg",
		"LoadImage2": "https://example.com/user.jpg",
	}
	require.NoError(t, applyFixedImageInputs(params, map[string]string{
		"LoadImage1": "https://example.com/fixed.jpg",
	}))
	require.Equal(t, "https://example.com/fixed.jpg", params["LoadImage1"])
	require.Equal(t, "https://example.com/user.jpg", params["LoadImage2"])
}

func TestApplyFixedImageInputsRejectsEmptyURL(t *testing.T) {
	require.Error(t, applyFixedImageInputs(map[string]string{}, map[string]string{
		"LoadImage1": " ",
	}))
}
