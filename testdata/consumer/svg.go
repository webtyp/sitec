//go:build !wasm

package consumer

import (
	"webtyp.com/svg"
	"webtyp.com/svg/sprite"
)

type Icons struct{}

func (i *Icons) IconSvg() *sprite.Sprite {
	return sprite.NewSprite(
		sprite.Define(svg.Icon("consumer-icon"), "0 0 24 24", sprite.Path("M0 0h24v24H0z")),
	)
}
