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

Un proyecto declara su icono con `Favicon()` en su paquete de configuración (`!wasm`):

```go
//go:embed logo.png
var logo []byte

func (b *Brand) Favicon() favicon.Source {
    return favicon.Source{Raster: logo}
}
```

`sitec` deriva el juego completo vía `webtyp.com/image/favicon` (`icon-32.png`, `icon-192.png`, `apple-touch-icon.png`, `favicon.ico` y `favicon.svg` si se provee SVG) y emite un `<link>` por cada archivo con `Rel`.

`sitec` **no sanea** el SVG que reciba: un SVG de un tercero se limpia antes con `webtyp.com/svg/sanitize`. El de un proyecto es suyo y es de confianza.

## Aplicación Web Progresiva (PWA)

Un proyecto se convierte en PWA declarando `Favicon()` (con un logo de al menos 512 px) y `PWA()` en su paquete raíz (`!wasm`):

```go
func (a *App) Favicon() favicon.Source {
    return favicon.Source{Raster: logo} // logo >= 512x512
}

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

Los archivos grandes que descarga el código del navegador (pesos de modelos, cachés) se declaran con `Artifacts()` en el paquete raíz (`!wasm`), junto a `Favicon()` y `PWA()`:

```go
//go:build !wasm

func Artifacts() []artifacts.Source {
	return []artifacts.Source{{ID: "decider-0.8b", Version: "q4-2026-09",
		File: "models/decider.wtypw", Needs: device.Requirement{MinTier: device.TierSIMD}}}
}
```

`sitec` mide cada archivo (tamaño y SHA-256, sin cargarlo en memoria), escribe `/artifacts.json` y, en `Build`, coloca cada archivo bajo `/artifacts/` con un enlace duro (o una copia por streaming). Nunca se empaquetan ni se precachean; `server/httpd` los sirve con `Cache-Control: no-store` (vía `pwa.CacheControl`). En desarrollo, las descargas se prueban tras un `Build` de release.

## Estado

En construcción.
