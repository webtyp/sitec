# Strict CSP Asset Pipeline (`sitec`)

Context: `sitec` emits an HTML shell free of inline `<script>`, `<style>`, `style=`, and `<svg>` so `server/httpd` can enforce a strict hash-based `Content-Security-Policy` with `require-trusted-types-for 'script'`.

```mermaid
flowchart TD
    Mods["SSR Modules & Local Assets"] --> Sprite["renderSprite() -> sprite.<hash>.svg"]
    Mods --> Lang["BundleTranslations() -> lang.<hash>.json"]
    Mods --> CSS["Minify style.css -> sriSHA384()"]
    Mods --> JS["Minify script.js -> sriSHA384()"]
    Sprite --> SetURL["svg.SetSpriteURL(spriteURL)"]
    SetURL --> Pages["emitPages() without inline sprite"]
    Sprite --> HTML["index.html <head>:\n<meta name='webtyp-sprite'>\n<meta name='webtyp-lang'>\n<link rel='stylesheet' integrity='sha384-...'>\n<script src integrity='sha384-...'>"]
    Lang --> HTML
    CSS --> HTML
    JS --> HTML
    JS --> Manifest["csp.json: {script_hashes: ['sha384-...']}"]
```

## Walkthrough

1. Merged SVG symbols render as `<svg xmlns="http://www.w3.org/2000/svg">...</svg>` into an external hashed artifact `sprite.<hash>.svg` and `svg.SetSpriteURL` points SSR pages to it.
2. Merged translations JSON writes into `lang.<hash>.json` (no `<script type="application/json">` in HTML).
3. Final minified CSS and JS bytes are hashed with SHA-384 (`sha384-<base64>`), written as `integrity="..." crossorigin="anonymous"` in `index.html`, and the script hash is emitted to `csp.json` for `server/httpd`.
