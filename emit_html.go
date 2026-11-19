package sitec

import "strings"

// NewHtmlHandler creates an HTML asset handler using the provided output filename
func NewHtmlHandler(ac *Config, outputName, cssURL, jsURL, faviconURL string) *asset {
	af := newAssetFile(outputName, "text/html", ac, nil)
	_ = faviconURL

	openHTML, closeHTML := buildHTMLShell(htmlShellOptions{
		cssURL: cssURL,
		jsURL:  jsURL,
	})
	af.contentOpen = append(af.contentOpen, &ContentFile{
		Path:    "index-open.html",
		Content: openHTML,
	})

	// El HTML de los módulos (contentMiddle) debe caer DENTRO del punto de
	// montaje `#app`, para que el servidor sirva ya el markup renderizado y el
	// primer Render() del WASM lo reemplace sin salto visual.
	// Se siembra como primera entrada de contentMiddle; Path "!app" ordena
	// antes que cualquier nombre de módulo real (rutas de imports Go).
	af.contentMiddle = append(af.contentMiddle, &ContentFile{
		Path:    "!app",
		Content: []byte(`<div id="app">`),
	})

	// default marcador de cierre index HTML
	af.contentClose = append(af.contentClose, &ContentFile{
		Path:    "index-close.html",
		Content: closeHTML,
	})

	return af
}

// parseExistingHtmlContent analiza un archivo HTML existente para identificar
// las secciones de apertura y cierre, considerando dónde deben insertarse los módulos
func parseExistingHtmlContent(content string) (openContent, closeContent string) {
	// Buscar un marcador explícito de comentario
	if i := strings.Index(content, "<!-- MODULES_PLACEHOLDER -->"); i != -1 {
		return content[:i], content[i+len("<!-- MODULES_PLACEHOLDER -->"):]
	}

	// Buscar un marcador de plantilla Go
	if i := strings.Index(content, "{{.Modules}}"); i != -1 {
		return content[:i], content[i+len("{{.Modules}}"):]
	}

	lines := strings.Split(content, "\n")
	var splitIndex int

	// 1. Buscar dentro de un tag <main> si existe
	inMain := false
	for i, line := range lines {
		lineLower := strings.ToLower(strings.TrimSpace(line))

		if strings.Contains(lineLower, "<main") {
			inMain = true
			continue
		}

		if inMain && strings.Contains(lineLower, "</main>") {
			// Colocar el índice antes del cierre de main para que los módulos
			// se inserten dentro del tag main
			splitIndex = i
			break
		}
	}

	// 2. Si no se encontró dentro de <main>, buscar antes del primer <script>
	if splitIndex == 0 {
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), "<script") {
				splitIndex = i
				break
			}
		}
	}

	// 3. Si no hay <script>, buscar antes de </body>
	if splitIndex == 0 {
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), "</body>") {
				splitIndex = i
				break
			}
		}
	}

	// Si todavía no tenemos un punto, usar el final
	if splitIndex == 0 {
		splitIndex = len(lines)
	}

	return strings.Join(lines[:splitIndex], "\n"), strings.Join(lines[splitIndex:], "\n")
}
