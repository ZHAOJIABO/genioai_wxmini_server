//go:build markdown
// +build markdown

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	html1 "github.com/alecthomas/chroma/formatters/html"
	"github.com/yuin/goldmark"
	highlight "github.com/yuin/goldmark-highlighting"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	html2 "github.com/yuin/goldmark/renderer/html"
)

func markdownToHTML(markdown []byte) (string, error) {
	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // 支持GitHub风格的markdown扩展
			highlight.NewHighlighting(
				highlight.WithStyle("github"),
				highlight.WithFormatOptions(
					html1.WithLineNumbers(true),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html2.WithHardWraps(),
			html2.WithXHTML(),
			html2.WithUnsafe(),
		),
	)

	if err := md.Convert(markdown, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	html, err := markdownToHTML(body)
	if err != nil {
		http.Error(w, "Failed to convert Markdown to HTML", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintln(w, appendHtmlHeadAndFoot(html))
}

const htmlHead = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji";
            line-height: 1.3;
        }
        p {
            margin-top: unset;
        }
        h3 {
            font-weight: bold;
            font-size: 1em;
            margin-top: unset;
        }
        ul {
            padding-left: 1.2em;
            list-style-type: disc;
        }
        ol {
            padding-left: 1.2em;
            list-style-type: disc;
        }
        code {
            background-color: #f6f8fa;
            border-radius: 3px;
            font-family: SFMono-Regular, Consolas, "Liberation Mono", Menlo, Courier, monospace;
            padding: 0.5em 0.4em;
            font-size: 85%;
        }
        pre {
            background-color: #f6f8fa;
            padding: 10px;
            border-radius: 6px;
            overflow: auto;
        }
        pre code {
            background: none;
            padding: 0;
            font-size: 80%;
            white-space: pre-wrap; /* 保留空格和换行符，并允许自动换行 */
            word-wrap: break-word; /* 支持长单词换行 */
        }
		pre code .row {
            white-space:pre;
            user-select:none;
            margin-right:0.4em;
            padding:0 0.4em 0 0.4em;
            color:#7f7f7f
        }
    </style>
</head>
<body>`

const htmlFoot = `</body></html>`

func appendHtmlHeadAndFoot(html string) string {
	html = strings.Replace(html, "style=\"background-color:#fff;\"><code>", "><code>", -1)
	html = strings.Replace(html, "style=\"white-space:pre;user-select:none;margin-right:0.4em;padding:0 0.4em 0 0.4em;color:#7f7f7f\"", "class=\"row\"", -1)

	return htmlHead + html + htmlFoot
	return strings.Replace(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Markdown to HTML</title>
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/github-markdown-css/4.0.0/github-markdown.min.css">
</head>
<body class="markdown-body">
    {{ .Content }}
</body>
</html>`, "{{ .Content }}", html, -1)
	return html
	//return htmlHead + html + htmlFoot
}

func main() {
	http.HandleFunc("/convert", handler)
	fmt.Println("Server is running at http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Failed to start server:", err)
	}
}
