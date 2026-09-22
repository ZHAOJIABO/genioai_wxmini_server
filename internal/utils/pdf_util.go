package utils

import (
	"fmt"
	"log"

	"code.sajari.com/docconv"
)

// test function to extract text from a PDF file
func ExtractPDFText(filePath string) string {
	res, err := docconv.ConvertPath(filePath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res)
	return res.Body
}
