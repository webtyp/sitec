---
PLAN: "feat: un proyecto puede tener páginas estáticas y shell a la vez; solo el shell carga wasm"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 16431016144738541591
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> Fase F1b de
> [`CLIENT_PAYLOAD_STAGING_MASTER_PLAN.md`](https://github.com/webtyp/app-releases/blob/main/docs/CLIENT_PAYLOAD_STAGING_MASTER_PLAN.md).

# Plan — páginas estáticas y shell conviviendo, con el wasm solo donde hace falta

## 1. El problema, con números medidos

Hoy un proyecto es **una cosa o la otra**:

- declara `RenderHTML()` → un `index.html` en la raíz: el *shell* de una app
  WASM, con el `<script>` del bootstrap;
- declara `RenderPages()` → un sitio de varias páginas estáticas.

Los dos a la vez en `/` son un error explícito (`emit_core.go:365`:
*"ssr: page collision at /"*).

Consecuencia: **cualquier visitante anónimo descarga el binario completo para
ver una pantalla que no lo necesita**. Medido en una app real
(`veltylabs/mjosefa-cms`, TinyGo `-opt=z -no-debug -panic=trap`):

| | |
|---|---|
| `client.wasm` | 486.402 bytes (161.941 gzip) |
| de eso, los cuatro módulos de dominio | ~21 KB — **6,7 %** |
| maquinaria SPA (`dom`, `form`, `crudview`, `input`, `json`, `components`…) | ~68 % |
| la pantalla de acceso (`layout/login`) | **1.309 bytes** de código |

O sea: partir el binario no sirve (los módulos son ruido), y la pantalla que
el visitante ve no es el problema. El problema es **enviarle la maquinaria de
la SPA a una página que no la usa**. La solución no es comprimir: es no
mandarla.

## 2. La decisión — la toma el framework, no la app

Esto es deliberado y no debe volverse configurable (doctrina:
`CONSTRUCTION_HARNESS.md` — un framework existe para quitar decisiones, no
para ofrecerlas):

1. Un proyecto **puede declarar los dos modos a la vez**. Deja de ser un error.
2. Lo que `RenderPages()` emite es **estático**: HTML + CSS, **sin la etiqueta
   `<script>` del bootstrap**.
3. Lo que `RenderHTML()` emite —el shell— se monta en una ruta **fija,
   decidida aquí**: `/app/`. La app no la elige, no la configura y no la puede
   cambiar. Misma ruta en toda app webtyp, para que el tamaño y el
   comportamiento de todas se puedan razonar igual.
4. Cuando conviven, `/` pertenece a las páginas. La colisión desaparece porque
   ya no compiten por la misma ruta.

Precedente que esto sigue: Blazor (.NET 8) resuelve el mismo problema —un
runtime wasm pesado— con *render modes* por página: la página estática no
envía wasm, la interactiva sí. Next.js, Remix y SvelteKit hacen lo mismo por
ruta. **Ninguno condiciona el bundle a la sesión del visitante**, y por eso
este plan tampoco: se decide por lo que la página necesita.

## 3. Lo que ya está resuelto y no hay que tocar

- **Las rutas de assets ya se adaptan solas.** `RouteExtractedAssets`
  (`emit_core.go:212`) usa rutas **relativas** cuando no hay páginas y
  **absolutas desde `/`** cuando sí las hay. Con los dos modos conviviendo hay
  páginas, así que caen en el caso absoluto, que es el correcto para un shell
  montado en `/app/`. **No hay lógica nueva de rutas en este plan.**
- El comentario de esa misma función ya dice que el shell "puede montarse bajo
  cualquier prefijo": montarlo en `/app/` no lo rompe.
- La etiqueta del bootstrap se emite en un solo lugar (`emit_html.go:18`), así
  que "solo el shell la lleva" es un condicional en un punto, no una cacería.

## 4. El cambio

- `emit_core.go`: quitar el error de colisión en `/` **solo** para el caso
  `RenderHTML` + `RenderPages` (sigue siendo error que dos módulos declaren
  `RenderPages` con el mismo `Path`, y que dos declaren `RenderHTML`). El
  shell pasa a emitirse en `app/index.html` cuando el proyecto también tiene
  páginas; cuando **no** las tiene, se sigue emitiendo en la raíz exactamente
  como hoy — **no romper el caso de la app-shell sola**, que es el de todas
  las apps existentes.
- `emit_html.go`: la etiqueta `<script>` del bootstrap se inyecta **solo** en
  el shell. Una página de `RenderPages()` nunca la lleva.
- La ruta fija va en **una constante exportada** del paquete
  (`ShellPath = "/app/"`), única definición; nada de literales repetidos.

## 5. Lo que NO hace este plan

- **No decide nada por sesión.** Una página no sabe ni pregunta quién la pide.
- **No parte el binario** ni agrega carga diferida por módulo: medido, no se
  justifica (§1).
- **No toca `webtyp/js`** (el bootstrap en sí no cambia; cambia quién lo
  incluye).
- **No cambia `webtyp/auth`.** Su `PathAfterLogin` (hoy `"/"`) tendrá que
  apuntar al shell para que el login aterrice en la app, pero eso es otro
  repo y va en su propio plan. **Importante para el ejecutor:** no agregar
  aquí ninguna dependencia a `webtyp/auth` para "compartir" la constante — el
  acuerdo entre ambas se verifica en los tests de integración de una app, que
  es donde las dos ya se encuentran, no acoplando un compilador de sitios a
  una librería de autenticación.

## 6. Tests obligatorios

1. **Conviven**: un proyecto con un módulo que declara `RenderHTML()` y otro
   que declara `RenderPages()` con `Path: "/"` compila **sin error** y emite
   `index.html` (la página) y `app/index.html` (el shell).
2. **Solo el shell trae el bootstrap**: `app/index.html` contiene
   `<script src=` del JS principal; `index.html` **no** contiene ningún
   `<script src=` al bootstrap.
3. **Regresión del caso actual — el más importante**: un proyecto que declara
   solo `RenderHTML()` (sin páginas) sigue emitiendo `index.html` **en la
   raíz**, con rutas de assets **relativas**, byte por byte como hoy. Todas
   las apps existentes están en este caso.
4. **Regresión del sitio**: un proyecto solo con `RenderPages()` no cambia.
5. **Las colisiones que siguen siendo error**: dos módulos con `RenderPages()`
   en el mismo `Path`, y dos módulos con `RenderHTML()`.
6. **Assets absolutos al convivir**: con los dos modos, las referencias a
   CSS/JS/sprite en ambas salidas empiezan con `/`.

## 7. Criterios de aceptación

1. `go build ./...`, `go vet ./...` y `gotest ./...` en verde.
2. `grep -rn '"/app/"' --include="*.go" . | grep -v _test` → **una sola**
   línea: la definición de `ShellPath`.
3. Los seis tests de §6 pasan.
4. `grep -rn "webtyp.com/auth" go.mod` → vacío (ver §5).

| Etapa | Archivos | Acción |
|---|---|---|
| 1 | `emit_core.go` | `ShellPath`; permitir la convivencia; emitir el shell en `app/index.html` solo si hay páginas |
| 2 | `emit_html.go` | El `<script>` del bootstrap solo en el shell |
| 3 | tests del paquete | Los seis casos de §6 |
