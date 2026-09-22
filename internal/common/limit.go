package common

import "va_visionai_server/internal/constants"

func IsLimited(packageName string, os string) bool {
	if packageName == constants.ProjectIdVisionAI && constants.ANDROID == os {
		return true
	}
	return false
}
