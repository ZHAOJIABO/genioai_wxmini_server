package picture_generate

func MapComfyResolutionToMinimax(resolution string) string {
	switch resolution {
	case "SD":
		return "768P"
	case "HD":
		return "1080P"
	default:
		return resolution
	}
}
