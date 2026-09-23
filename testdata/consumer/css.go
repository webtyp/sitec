//go:build !wasm

package consumer

import "webtyp.com/css"

type Styles struct{}

func (s *Styles) RenderCSS() *css.Stylesheet {
	return css.NewStylesheet(
		css.Raw("body { margin: 0; background-color: #f0f0f0; }"),
	)
}
