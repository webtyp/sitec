# sitec
<img src="docs/img/badges.svg">

Compilador de sitio: toma un árbol de fuentes Go y produce la superficie
estática desplegable del sitio — hoja de estilos, bundle de scripts, sprite SVG,
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

## Estado

En construcción.
