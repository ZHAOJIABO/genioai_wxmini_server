package credit

import "va_visionai_server/conf"

// ImageGenerationCost applies the waiver after all pricing adjustments.
// Callers must classify images using server-side metadata, not client input.
// Retries pass memberFree=false to preserve their stored price when the switch is off.
func ImageGenerationCost(amount int, isImage, memberFree bool) int {
	if isImage && (conf.GlobalConfig.AmountConfig.FreeImageGeneration || memberFree) {
		return 0
	}
	return amount
}
