package app

import (
	ui "github.com/metaspartan/gotui/v5"
)

type DraculaPalette struct {
	Background  ui.Color
	CurrentLine ui.Color
	Selection   ui.Color
	Foreground  ui.Color
	Comment     ui.Color
	Cyan        ui.Color
	Green       ui.Color
	Orange      ui.Color
	Pink        ui.Color
	Purple      ui.Color
	Red         ui.Color
	Yellow      ui.Color
}

var Dracula = DraculaPalette{
	Background:  rgb(40, 42, 54),
	CurrentLine: rgb(68, 71, 90),
	Selection:   rgb(68, 71, 90),
	Foreground:  rgb(248, 248, 242),
	Comment:     rgb(98, 114, 164),
	Cyan:        rgb(139, 233, 253),
	Green:       rgb(80, 250, 123),
	Orange:      rgb(255, 184, 108),
	Pink:        rgb(255, 121, 198),
	Purple:      rgb(189, 147, 249),
	Red:         rgb(255, 85, 85),
	Yellow:      rgb(241, 250, 140),
}

var DraculaHex = map[string]string{
	"Primary": "#bd93f9",
	"Text":    "#f8f8f2",
	"Base":    "#282a36",
}

func IsDraculaTheme(theme string) bool {
	return theme == "dracula"
}

func GetDraculaHex(colorName string) string {
	if val, ok := DraculaHex[colorName]; ok {
		return val
	}
	return "#ffffff"
}

func GetDraculaPalette(name string) *DraculaPalette {
	if name == "dracula" {
		return &Dracula
	}
	return nil
}
