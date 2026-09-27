// Package pairing produit le lien et le QR code d'appairage lus par l'application Android.
package pairing

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"strconv"
	"strings"

	"rsc.io/qr"
)

// Info contient ce que l'application a besoin de connaître.
type Info struct {
	Name string
	Host string
	Port int
	MAC  string // facultatif
	Key  string
}

// Link construit le lien `wolagent://pair?v=1&n=…&h=…&p=…&m=…&k=…`.
func Link(info Info) string {
	params := []string{
		"v=1",
		"n=" + escape(info.Name),
		"h=" + escape(info.Host),
		"p=" + strconv.Itoa(info.Port),
	}
	if info.MAC != "" {
		params = append(params, "m="+escape(strings.ToUpper(info.MAC)))
	}
	params = append(params, "k="+escape(info.Key))
	return "wolagent://pair?" + strings.Join(params, "&")
}

// escape encode une valeur de paramètre ; « : » est conservé (valide dans une URL) pour un QR code plus petit.
func escape(value string) string { return strings.ReplaceAll(url.QueryEscape(value), "%3A", ":") }

// WriteTerminalQR dessine le QR code avec des demi-blocs Unicode (2 lignes du code par ligne de texte).
// Avec ansi=true, les couleurs noir/blanc sont forcées (indépendant du thème du terminal) ;
// sinon on suppose un terminal à fond sombre, sauf si invert=true.
func WriteTerminalQR(w io.Writer, text string, ansi, invert bool) error {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return err
	}
	const quiet = 2
	size := code.Size + 2*quiet
	dark := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		return x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)
	}
	var b strings.Builder
	for y := 0; y < size; y += 2 {
		for x := 0; x < size; x++ {
			top, bottom := dark(x, y), y+1 < size && dark(x, y+1)
			if ansi {
				// Caractère « ▀ » : couleur de texte = module du haut, fond = module du bas.
				fmt.Fprintf(&b, "\x1b[%d;%dm▀", fg(top), bg(bottom))
				continue
			}
			// Sans couleurs : on dessine les modules CLAIRS (fond sombre supposé).
			lightTop, lightBottom := top == invert, bottom == invert
			if y+1 >= size {
				lightBottom = false
			}
			switch {
			case lightTop && lightBottom:
				b.WriteString("█")
			case lightTop:
				b.WriteString("▀")
			case lightBottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		if ansi {
			b.WriteString("\x1b[0m")
		}
		b.WriteString("\n")
	}
	_, err = io.WriteString(w, b.String())
	return err
}

func fg(dark bool) int {
	if dark {
		return 30
	}
	return 97
}

func bg(dark bool) int {
	if dark {
		return 40
	}
	return 107
}

// WritePNG écrit le QR code dans une image PNG (alternative si le terminal l'affiche mal).
func WritePNG(w io.Writer, text string, scale int) error {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return err
	}
	const quiet = 4
	size := (code.Size + 2*quiet) * scale
	img := image.NewGray(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			mx, my := x/scale-quiet, y/scale-quiet
			black := mx >= 0 && my >= 0 && mx < code.Size && my < code.Size && code.Black(mx, my)
			if black {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return png.Encode(w, img)
}
