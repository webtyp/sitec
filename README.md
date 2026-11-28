# sitec
<img src="docs/img/badges.svg">

Compilador de sitio: toma un árbol de fuentes Go y produce la superficie
estática desplegable del sitio — hoja de estilos, bundle de scripts, sprite SVG (dentro del HTML, nunca como archivo aparte),
declaración de fuentes y shell HTML.

Corre hasta terminar y sale. Es un compilador, no un servidor ni un
renderizador — pensado para CI/CD tanto como para el arnés de desarrollo.

```
sitec              # ayuda, exit 0
sitec build -o dir # compila y escribe la salida
sitec check        # valida sin escribir nada (puerta de CI)
```

stdout entrega datos (manifiesto JSON); stderr entrega logs.

## Productores

`sitec` descubre funciones y métodos en los paquetes Go que devuelven los activos del sitio, como `RenderPages() []html.Page` y `RootCSS() *css.Stylesheet`.
También inspecciona las variables exportadas del paquete si sus valores implementan las interfaces de productores de páginas o shell (como `RenderPages()` o `RenderHTML()`), de modo que las variables de configuración de la app pueden renderizar páginas directamente.

```go
// config/brand.go — sitec calls RenderPages and RenderHTML on the value
var Brand = appkit.Brand{Name: "Acme", LogoMark: logo}
```

## Uso programático (Build de producción)

Para compilar y emitir la superficie estática desplegable en un pipeline de producción sin depender del demonio de desarrollo:

```go
package main

import "webtyp.com/sitec"

func main() {
	if err := sitec.Build(".", "web/public"); err != nil {
		panic(err)
	}
}
```

`sitec.Build` ejecuta el pipeline completo en una sola pasada, borra el contenido previo de la salida y escribe el árbol servible a disco con minificación activada por defecto.

> **Nota:** La compilación del binario Go WASM (`.wasm`) es responsabilidad del llamador (ej. invocando `go build` o `tinygo` para `GOOS=js GOARCH=wasm`). `Build` emite el runtime JS y enlaza el binario.

## Icono del sitio

El icono de un proyecto es un archivo, no código: `docs/logo/logo.png` (cuadrado, de al menos 512 px) y, si quiere, `docs/logo/logo.svg` junto a él. No hay nada que declarar. Un PNG que no es cuadrado o es más pequeño de lo necesario falla el build y nombra el archivo.

```
docs/logo/logo.png    ← obligatorio
docs/logo/logo.svg    ← opcional
```

Una aplicación (tiene `config/` o `web/<binario>/main.go`) sin icono lo ve una vez en el log con el enlace a la documentación; un módulo no lo ve nunca. La demo de un módulo recibe el logo de webtyp en memoria (`Config.FallbackFavicon`).

`sitec` deriva el juego completo en memoria vía `webtyp.com/image/favicon` (`icon-32.png`, `icon-192.png`, `apple-touch-icon.png`, `favicon.ico` y `favicon.svg` si se provee SVG) y emite un `<link>` por cada archivo con `Rel`.

`sitec` **no sanea** el SVG que reciba: un SVG de un tercero se limpia antes con `webtyp.com/svg/sanitize`. El de un proyecto es suyo y es de confianza.

## Aplicación Web Progresiva (PWA)

Un proyecto se convierte en PWA con un `docs/logo/logo.png` de al menos 512 px y `PWA()` en su paquete raíz (`!wasm`):

```go
func (a *App) PWA() pwa.Config {
    return pwa.Config{
        Name:            "Mi Aplicación",
        ShortName:       "App",
        ThemeColor:      "#0080ff",
        BackgroundColor: "#ffffff",
    }
}
```

En builds de producción (`ModeRelease`), `sitec` emite automáticamente `manifest.webmanifest`, inyecta los tags `<head>` y el script de registro del service worker, renombra los activos con su hash de contenido (`style.3f9a1c2b.css`) y genera `sw.js` con el manifiesto de precaché completo. En desarrollo (`ModeDev`), mantiene nombres fijos sin service worker.

## Artefactos pesados

Los archivos grandes que descarga el código del navegador (pesos de modelos, cachés) se declaran con `ArtifactSources()` en el paquete raíz (`!wasm`), junto a `PWA()`:

```go
//go:build !wasm

func ArtifactSources() []artifacts.Source {
	return []artifacts.Source{{ID: "decider-0.8b", Version: "q4-2026-09",
		File: "models/decider.wtypw", Needs: device.Requirement{MinTier: device.TierSIMD}}}
}
```

`sitec` mide cada archivo (tamaño y SHA-256, sin cargarlo en memoria), escribe `/artifacts.json` y, en `Build`, coloca cada archivo bajo `/artifacts/` con un enlace duro (o una copia por streaming). Nunca se empaquetan ni se precachean; `server/httpd` los sirve con `Cache-Control: no-store` (vía `pwa.CacheControl`). En desarrollo (`webtyp dev`) el `Compiler` escribe el mismo `/artifacts.json` y `sitec/serve` entrega cada archivo desde donde está, con Range, sin copiarlo.

## Web Workers

El código pesado (PDF, modelos, inferencia) corre fuera del hilo principal, en un Web Worker: es lo único que se compila a WebAssembly (la página y el edge son ES Modules de `goesm`). `sitec.FindWebWorkers` los encuentra solo:
- los de la app: cada `web/webworkers/<nombre>/main.go` (ej. `web/webworkers/echo/main.go` → worker `echo`);
- los de las librerías: cada librería que el build de la app alcanza y trae `webworker/main.go` (ej. `webtyp.com/pdf/webworker` → `/pdf.wasm` + `/pdf.worker.js`). La app no escribe nada. Dos workers con el mismo nombre son un error que nombra ambos.

Cada `main.go` declara su compilador en la primera línea, y sin ella `sitec` falla mostrando las dos opciones:
- `//go:build wasm && tinygo` — binario chico y SIMD. En release se compila dos veces: **SIMD** (`+simd128`, `-opt=2`, D16) y **plana** (`-opt=2`); en desarrollo una vez, con SIMD, como `<nombre>.wasm`.
- `//go:build wasm && !tinygo` — la librería estándar completa de Go. Go no tiene SIMD para WebAssembly: se compila una vez y los dos scripts la cargan.

El script de cada worker lleva el `wasm_exec.js` de su compilador (`js.WebWorker(nombre, url, js.RuntimeTinyGo | js.RuntimeGo)`).

Produce cuatro archivos por worker:
- El script arrancador: `<name>.worker.js` (apunta a la versión plana).
- El script arrancador SIMD: `<name>.simd.worker.js` (apunta a la versión SIMD).
- Los binarios WASM hasheados: `<name>.<hash>.wasm` y `<name>.simd.<hash>.wasm`.

La aplicación lo arranca así en su código de página:
```go
agentworker.Start(Scripts{
    Plain: "/echo.worker.js",
    SIMD:  "/echo.simd.worker.js",
}, ...)
```

## Estado

En construcción.
