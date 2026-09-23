---
PLAN: "feat: one-shot production build — sitec emits a deployable web/public without the dev daemon"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — un build de producción que no dependa del daemon

## El problema, medido

`veltylabs/mjosefa-cms` es la primera app webtyp que va a producción y **hoy no
se puede desplegar**: su directorio servible (`web/public/`: el `.wasm`, el
runtime JS, el CSS generado, el sprite SVG, el `index.html`) lo produce
únicamente el daemon de desarrollo. Comprobado:

- El CLI `webtyp` expone `dev`, `mcp`, `test`, `status`, `stop`, `update` — **no
  hay subcomando de build**.
- `web/public/` está en el `.gitignore` de la app: es artefacto generado, no
  fuente, y no viaja en el repo.
- El binario del servidor sí sale con `go build ./web` — lo que falta es el
  lado cliente.

El resultado práctico: para desplegar habría que correr el daemon de desarrollo
en el servidor, que es exactamente lo que un daemon de desarrollo no debe hacer.

## Por qué esta pieza y no la app

`sitec` ya ES el compilador de assets: `sitec.New(root)`, `sitec.Config`,
`sitec.Compiler`, `sitec.NewOsFS()`. `webtyp/app` solo lo envuelve
(`AssetsHandler`) para observar cambios y recompilar. Un build de una sola
pasada no necesita nada de esa orquestación: necesita el compilador y un
directorio de salida.

Y es glue que **toda** app webtyp repetiría igual — por la regla de la skill
**api-design** ("the glue is written once, in the library that owns it"), vive
acá, no copiado en el `cmd/` de cada aplicación.

## Design gate (skill: api-design)

### 1. Prior art

- **Next.js / Nuxt**: `dev` y `build` son el mismo pipeline con un flag; el
  artefacto de `build` es un directorio servible por cualquier servidor
  estático.
- **Vite**: `vite` (dev, en memoria) vs `vite build` (a disco, minificado) —
  misma configuración, dos salidas.
- **Hugo**: `hugo server` vs `hugo`; el segundo escribe `public/` y es lo que se
  despliega.

Los tres separan "observar y servir en memoria" de "emitir a disco una vez", y
los tres comparten configuración entre ambos modos. Este plan es esa misma
familia: el `Compiler` ya existe y ya sabe emitir; lo que falta es la entrada
de una sola pasada.

### 2. La prueba del nombre novato

`Build` — la palabra que usan Next, Vite, Hugo, Go y Make para exactamente esto.
No `Emit`, no `Generate`, no `Compile` (que en este repo ya nombra el paso
interno por asset): un junior que lee `sitec.Build(...)` sabe qué hace sin abrir
la implementación.

### 3. El libro de complejidad

```
Conceptos que aprender              +1   (Build, una función)
Archivos que tocar para hacer X     −1   (hoy: ninguno sirve; hay que correr el daemon)
Líneas en el call site              +1   (una llamada desde cmd/ de la app, o el CLI)
Formas de hacer lo mismo             0   (no existe otra forma hoy)
```

### 4. Dónde vive

`webtyp/sitec`: es el repo que posee `Compiler` y las reglas de emisión. La app
consumidora no re-crea nada; `webtyp/app` tampoco cambia — puede seguir usando
su `AssetsHandler`, o migrar a `Build` después, fuera de este plan.

### 5. Qué borra este cambio

No borra código: cubre un hueco. Lo que elimina es la **necesidad de correr el
daemon de desarrollo en un servidor de producción**, que hoy es la única vía.

## Qué construir

Una función exportada, de una pasada, sin watchers, sin servidor, sin estado:

```go
// Build compiles every asset of the project rooted at rootDir and writes the
// servable tree to outDir. One pass, no watching, no server: this is the
// entry point a release pipeline calls.
func Build(rootDir, outDir string, opts ...Option) error
```

Decisiones ya tomadas — el ejecutor no elige:

- **Minificación ENCENDIDA por defecto** en `Build` (el `Compiler` ya expone
  `SetMinifyEnabled`). Es la diferencia de fondo entre dev y release, y el
  default correcto para un artefacto que se descarga por red. Si hace falta
  apagarla, que sea una `Option` explícita y greppable, nunca un booleano
  posicional (regla 2 de la skill: sin parámetros booleanos).
- **`NewOsFS()`**, nunca `NewMemFS()`: el artefacto va a disco.
- **Falla ruidosa**: cualquier asset que no compile aborta con error; `Build`
  no emite un árbol parcial ni deja un `web/public/` a medio escribir que un
  despliegue tomaría por bueno.
- **`outDir` se crea si no existe** y su contenido previo se reemplaza; un
  residuo de un build anterior no debe viajar al servidor.
- **No toca el `.wasm` de Go**: compilar `GOOS=js GOARCH=wasm` es trabajo del
  toolchain de Go, no de este paquete. `Build` sí debe colocar el runtime JS y
  referenciar el `.wasm` como ya lo hace el `Compiler` (ver `SetWasm`), y su
  documentación debe decir explícitamente que el `.wasm` lo produce quien
  llama. Un `Build` que además invocara `go build` estaría asumiendo dos
  responsabilidades.

## Pasos

1. **Leer primero** `emit_core.go` y el resto de `emit_*.go` para ver qué hace
   hoy el `Compiler` completo (fuentes, CSS, SVG, HTML, wasm runtime) y cuál es
   el orden real de emisión. `Build` debe producir **el mismo árbol** que el
   daemon produce hoy — no una variante.
2. Implementar `Build` en un archivo nuevo `build.go`, reutilizando
   `sitec.New`/`Config`/`Compiler`. Sin duplicar lógica de emisión: si algo no
   es alcanzable desde la superficie actual, expón lo mínimo, no copies.
3. `Option` tipada para lo que de verdad varía (minificación). Nada de `...any`,
   nada de booleanos posicionales.
4. **Test de forma de consumidor, dentro de esta librería** (lo exige la skill
   antes de publicar): un proyecto de prueba mínimo en `testdata/` con al menos
   un `css.go` y un `svg.go`, `Build` sobre un directorio temporal, y
   afirmaciones sobre los archivos emitidos — que existen, que no están vacíos,
   y que una segunda corrida sobre el mismo `outDir` no deja residuo de la
   primera.
5. `README.md`: una fila en la tabla "quiero X → uso Y" con `Build`, y el
   ejemplo de tres líneas de un `cmd/build/main.go` de una app.

## Criterios de aceptación

- `gotest` en verde (nunca `go test`).
- `Build` es la única superficie pública nueva, más su `Option`; nada más se
  exporta.
- El test de consumidor corre sobre el `Compiler` real y el FS real, no sobre
  dobles.
- La documentación de `Build` dice, en su propio comentario, que el `.wasm` es
  responsabilidad de quien llama.
- `grep -rn "TODO\|FIXME" --include='*.go' .` sin entradas nuevas.

## Fuera de alcance

- No tocar `webtyp/app` ni su `AssetsHandler`.
- No agregar un `cmd/` a este repo: la entrada de línea de comandos se decide
  después, y una función exportada ya desbloquea a la app.
- No compilar wasm ni invocar el toolchain de Go.

## Etapas

| # | Etapa | Entregable |
|---|---|---|
| 1 | Leer `emit_*.go` y fijar el orden de emisión real | — |
| 2 | `build.go`: `Build(rootDir, outDir, opts...)` + `Option` de minificación | código |
| 3 | Test de forma de consumidor con `testdata/` y `outDir` temporal | test |
| 4 | `README.md`: fila en la tabla + ejemplo de `cmd/build/main.go` | docs |
