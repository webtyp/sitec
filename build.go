package sitec

import (
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/fmt"
	"webtyp.com/image/min"
	"webtyp.com/router/routescan"
)

const (
	DefaultOutputDir    = "web/public"
	DefaultImageQuality = 82
)

// Mode decides what artifact Build produces.
type Mode uint8

const (
	// ModeRelease is the deliverable: WASM via TinyGo, minified.
	ModeRelease Mode = iota
	// ModeDev is the development cache: fast compilation, unminified.
	ModeDev
)

// Site es lo que un módulo RAÍZ declara sobre el sitio que produce.
//
// Declararla convierte al proyecto en un sitio estático: el entregable es el
// directorio de salida y RenderPages() es el dueño del index.html. Un proyecto
// sin RenderSite() es una aplicación y su index.html es el shell de arranque
// del WASM.
//
// Lo que declara RenderSite() manda sobre lo que traiga BuildConfig. El
// proyecto es la autoridad sobre sí mismo; BuildConfig es el afinado del
// llamador. Cuando ambos traen valor y difieren, se registra un aviso con los
// dos valores y se aplica el del proyecto.
type Site struct {
	// URL es la URL pública del sitio. Habilita sitemap.xml y las URL
	// canónicas absolutas. Vacía ⇒ no se emite sitemap.
	URL string `json:"url"`

	// StaticAssets son rutas relativas a la raíz del módulo que se copian
	// verbatim a la salida. Para lo que NO pasa por el pipeline de imágenes:
	// SVG de marca, PDF, robots.txt.
	//
	// Un archivo o directorio declarado y ausente es un ERROR de build, no un
	// aviso: un logo que falta en producción se descubre demasiado tarde.
	StaticAssets []string `json:"static_assets"`
}

// BuildConfig contains the site build settings.
type BuildConfig struct {
	RootDir        string // Root directory of the module (where go.mod lives). Required.
	Mode           Mode
	OutputDir      string // Relative to RootDir. Empty => DefaultOutputDir.
	SiteURL        string // Enables sitemap.xml and absolute canonical URLs.
	AppName        string
	StaticAssets   []string // Declared static assets relative to RootDir copied verbatim.
	ImageQuality   int      // 0 => DefaultImageQuality
	AssetLibraries []string // Style libraries whose importers must declare a producer.
	Log            func(...any)
}

// ErrRouteCollides is returned when a declared route has the same path as a
// produced static artifact. The artifact is served first, so the route would be
// unreachable with no error anywhere.
const ErrRouteCollides = "route %s %s (routes/routes.go:%d) collides with the static asset %s — the asset is served first and the route would never run"

const symbolParamStart = "{"

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func splitPath(p string) []string {
	norm := normalizePath(p)
	if norm == "/" {
		return []string{}
	}
	return strings.Split(norm[1:], "/")
}

func routeMatchesArtifact(routeSegs, artSegs []string) bool {
	if len(routeSegs) != len(artSegs) {
		return false
	}
	for i := 0; i < len(routeSegs); i++ {
		r := routeSegs[i]
		a := artSegs[i]
		if strings.HasPrefix(r, symbolParamStart) {
			continue
		}
		if r != a {
			return false
		}
	}
	return true
}

func checkRouteCollisions(routes []routescan.Decl, artifacts []Artifact) error {
	if len(routes) == 0 || len(artifacts) == 0 {
		return nil
	}

	type parsedArtifact struct {
		origPath string
		segments []string
	}

	parsedArts := make([]parsedArtifact, 0, len(artifacts))
	for _, art := range artifacts {
		parsedArts = append(parsedArts, parsedArtifact{
			origPath: art.Path,
			segments: splitPath(art.Path),
		})
	}

	for _, decl := range routes {
		routeSegs := splitPath(decl.Path)
		for _, art := range parsedArts {
			if routeMatchesArtifact(routeSegs, art.segments) {
				return fmt.Err(fmt.Sprintf(ErrRouteCollides, decl.Method, decl.Path, decl.Line, art.origPath))
			}
		}
	}
	return nil
}

// Output es el resultado de un Build COMPLETO: los artefactos producidos,
// listos para volcarse a un FS (WriteTo) o servirse desde Artifacts().
type Output struct {
	c      *Compiler
	routes []routescan.Decl
}

// Routes returns the routes the project declared in routes/routes.go, in
// source order. Empty when the project declares none.
func (s *Output) Routes() []routescan.Decl {
	if s == nil {
		return nil
	}
	return s.routes
}

// Artifacts returns all produced artifacts.
func (s *Output) Artifacts() []Artifact {
	if s == nil || s.c == nil {
		return nil
	}
	return s.c.List()
}

// An artifact's Path is a URL ("/style.css", "/", "/acerca/") — the URL is
// the identity. diskPath resolves it to a real file under OutputDir
// ("web/public/especialidades/oftalmologia/index.html"). Formula shared with
// Compiler.artifactDiskPath (emit_flush.go).
func (s *Output) diskPath(art Artifact) string {
	return s.c.artifactDiskPath(art.Path)
}

// WriteTo writes the built site artifacts to the given FS.
func (s *Output) WriteTo(fs FS) error {
	if s == nil || s.c == nil {
		return fmt.Err("sitec: WriteTo called on nil Output")
	}
	for _, art := range s.Artifacts() {
		if err := fs.Write(s.diskPath(art), art.Content, art.Mediatype); err != nil {
			return err
		}
	}
	return nil
}

// Build executes the entire build pipeline in memory. It does not write to the output disk.
func Build(cfg BuildConfig) (*Output, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Err("sitec: RootDir is required")
	}

	root, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Err("sitec: error resolving RootDir:", err)
	}

	if err := ValidateProject(root); err != nil {
		return nil, err
	}

	e := New(root)
	if cfg.Log != nil {
		e.SetLog(cfg.Log)
	}
	// Solo se sobrescribe si el llamador declaró su propia lista: pasar nil no
	// debe apagar la comprobación que New() deja encendida.
	if len(cfg.AssetLibraries) > 0 {
		e.SetAssetLibraries(cfg.AssetLibraries)
	}

	if _, err := os.Stat(filepath.Join(root, "web", "client.go")); err == nil {
		e.SetWasmBuilder(NewDefaultWasmBuilder(cfg.Mode == ModeDev))
	}

	all, err := e.ExtractAll()
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Err(msgEmptyExtraction())
	}

	outDir := cfg.OutputDir
	if outDir == "" {
		outDir = DefaultOutputDir
	}

	imgQuality := cfg.ImageQuality
	if imgQuality == 0 {
		imgQuality = DefaultImageQuality
	}

	c := NewCompiler(&Config{
		OutputDir: outDir,
		RootDir:   root,
		AppName:   cfg.AppName,
		SiteURL:   cfg.SiteURL,
		DevMode:   cfg.Mode == ModeDev,
	})
	if cfg.Log != nil {
		c.SetLog(cfg.Log)
	}
	c.SetFS(NewMemFS())

	imgHandler := min.New(&min.Config{
		RootDir:   root,
		OutputDir: filepath.Join(outDir, "img"),
		Quality:   imgQuality,
	})
	if cfg.Log != nil {
		imgHandler.SetLog(cfg.Log)
	}
	imgHandler.SetFinder(e.Finder())
	c.SetImageProcessor(imgHandler)

	wb := e.WasmBuilder()
	if wb != nil {
		wasmOut, err := wb.Build(root)
		if err != nil {
			return nil, err
		}
		c.SetWasm(wasmOut.Filename, wasmOut.Runtime)
		if err := c.Write(wasmOut.Filename, wasmOut.Binary, "application/wasm"); err != nil {
			return nil, err
		}
	}

	if err := c.RouteExtractedAssets(all); err != nil {
		return nil, err
	}

	if err := imgHandler.LoadImages(); err != nil {
		return nil, err
	}

	if err := c.PublishImages(); err != nil {
		return nil, err
	}

	// Activos estáticos: los de RenderSite() (el proyecto manda) más los de
	// BuildConfig, unidos sin duplicados en un solo recorrido.
	var staticAssets []string
	if c.site != nil {
		staticAssets = append(staticAssets, c.site.StaticAssets...)
	}
	staticAssets = append(staticAssets, cfg.StaticAssets...)
	staticAssets = dedupeStrings(staticAssets)
	if err := copyStaticAssets(c, root, staticAssets); err != nil {
		return nil, err
	}

	routes, err := routescan.Scan(root)
	if err != nil {
		return nil, err
	}

	out := &Output{c: c, routes: routes}
	if err := checkRouteCollisions(routes, out.Artifacts()); err != nil {
		return nil, err
	}

	return out, nil
}

// LoadStaticAssets copia a la salida los activos declarados por RenderSite().
// Separado de RouteExtractedAssets porque un activo estático no participa en
// la cascada de CSS ni en el sprite: solo se copia.
//
// Es el camino que usa el demonio de desarrollo: Compiler conoce el sitio del
// raíz y puede copiar lo que declara sin pasar por Build(). Build() la usa
// igualmente, unida a BuildConfig.StaticAssets y sin duplicados.
func (c *Compiler) LoadStaticAssets() error {
	c.mu.Lock()
	site := c.site
	rootDir := ""
	if c.Config != nil {
		rootDir = c.Config.RootDir
	}
	c.mu.Unlock()

	if site == nil {
		return nil
	}
	return copyStaticAssets(c, rootDir, site.StaticAssets)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Check validates the project extraction and returns the list of extracted module names.
func Check(rootDir string, log func(...any)) ([]string, error) {
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Err("sitec: error resolving RootDir:", err)
	}

	if err := ValidateProject(root); err != nil {
		return nil, err
	}

	e := New(root)
	if log != nil {
		e.SetLog(log)
	}

	all, err := e.ExtractAll()
	if err != nil {
		return nil, err
	}

	var modules []string
	for _, a := range all {
		if a != nil {
			modules = append(modules, a.ModuleName)
		}
	}

	routes, err := routescan.Scan(root)
	if err != nil {
		return nil, err
	}

	if len(routes) > 0 {
		// Build an ephemeral Compiler output to obtain produced static artifacts
		outDir := DefaultOutputDir
		c := NewCompiler(&Config{
			OutputDir: outDir,
			RootDir:   root,
		})
		c.SetFS(NewMemFS())
		if err := c.RouteExtractedAssets(all); err == nil {
			if err := checkRouteCollisions(routes, c.List()); err != nil {
				return nil, err
			}
		}
	}

	return modules, nil
}

func copyStaticAssets(c *Compiler, rootDir string, staticAssets []string) error {
	for _, entry := range staticAssets {
		srcPath := filepath.Join(rootDir, entry)
		info, err := os.Stat(srcPath)
		if err != nil {
			return fmt.Err(msgStaticAbsent(entry))
		}

		if info.IsDir() {
			err = filepath.Walk(srcPath, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(rootDir, p)
				if err != nil {
					return err
				}
				content, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				mt := detectMediaType(p)
				return c.Write(rel, content, mt)
			})
			if err != nil {
				return err
			}
		} else {
			content, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			mt := detectMediaType(srcPath)
			if err := c.Write(entry, content, mt); err != nil {
				return err
			}
		}
	}
	return nil
}

func detectMediaType(p string) string {
	switch filepath.Ext(p) {
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	case ".svg":
		return "image/svg+xml"
	case ".html":
		return "text/html"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	case ".wasm":
		return "application/wasm"
	default:
		return "text/plain"
	}
}
