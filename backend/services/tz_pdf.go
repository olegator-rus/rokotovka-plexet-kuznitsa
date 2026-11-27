package services

import (
	"github.com/ledongthuc/pdf"
)

// Глифы страницы PDF с координатами. Свой обход потока содержимого поверх pdf.Interpret, а не Page.Content():
// библиотека после каждого TJ рисует псевдоглиф "\n" через кодировку шрифта, и в шрифтах LibreOffice байт 0x0A
// превращается в «№» в конце строк. Остальное повторяет Page.Content() (BSD-3, github.com/ledongthuc/pdf).

type tzMatrix [3][3]float64

var tzIdent = tzMatrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func (x tzMatrix) mul(y tzMatrix) tzMatrix {
	var z tzMatrix
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				z[i][j] += x[i][k] * y[k][j]
			}
		}
	}
	return z
}

type tzGState struct {
	Tc, Tw, Th, Tl, Tfs, Trise float64
	Tf                         pdf.Font
	enc                        pdf.TextEncoding
	Tm, Tlm, CTM               tzMatrix
}

// tzGlyphs — глифы страницы. Паника библиотеки на битом потоке — у вызывающего (recover в tzPDFText).
func tzGlyphs(p pdf.Page) []pdf.Text {
	strm := p.V.Key("Contents")
	if strm.IsNull() {
		return nil
	}
	g := tzGState{Th: 1, CTM: tzIdent, Tm: tzIdent, Tlm: tzIdent}
	var gstack []tzGState
	var text []pdf.Text
	fonts := map[string]*pdf.Font{}
	show := func(s string) {
		if g.enc == nil || s == "" {
			return
		}
		runes := []rune(g.enc.Decode(s))
		step := 1
		if len(runes) > 0 && len(s) == 2*len(runes) {
			step = 2 // двухбайтовые коды (Identity-H)
		}
		base := g.Tf.BaseFont()
		for i, ch := range runes {
			code := 0
			if i*step+step <= len(s) {
				for k := 0; k < step; k++ {
					code = code<<8 | int(s[i*step+k])
				}
			}
			w0 := g.Tf.Width(code)
			if w0 <= 0 {
				w0 = 500 // ширины нет в словаре шрифта: средняя, чтобы глифы не слиплись в одну точку
			}
			trm := tzMatrix{{g.Tfs * g.Th, 0, 0}, {0, g.Tfs, 0}, {0, g.Trise, 1}}.mul(g.Tm).mul(g.CTM)
			text = append(text, pdf.Text{Font: base, FontSize: trm[0][0], X: trm[2][0], Y: trm[2][1],
				W: w0 / 1000 * trm[0][0], S: string(ch)})
			tx := w0/1000*g.Tfs + g.Tc
			if step == 1 && code == ' ' {
				tx += g.Tw
			}
			tx *= g.Th
			g.Tm = tzMatrix{{1, 0, 0}, {0, 1, 0}, {tx, 0, 1}}.mul(g.Tm)
		}
	}
	nextLine := func() {
		g.Tlm = tzMatrix{{1, 0, 0}, {0, 1, 0}, {0, -g.Tl, 1}}.mul(g.Tlm)
		g.Tm = g.Tlm
	}
	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		num := func(i int) float64 {
			if i < len(args) {
				return args[i].Float64()
			}
			return 0
		}
		switch op {
		case "q":
			gstack = append(gstack, g)
		case "Q":
			if k := len(gstack) - 1; k >= 0 {
				g, gstack = gstack[k], gstack[:k]
			}
		case "cm":
			if len(args) == 6 {
				var m tzMatrix
				for i := 0; i < 6; i++ {
					m[i/2][i%2] = num(i)
				}
				m[2][2] = 1
				g.CTM = m.mul(g.CTM)
			}
		case "BT":
			g.Tm, g.Tlm = tzIdent, tzIdent
		case "Tc":
			g.Tc = num(0)
		case "Tw":
			g.Tw = num(0)
		case "Tz":
			g.Th = num(0) / 100
		case "TL":
			g.Tl = num(0)
		case "Ts":
			g.Trise = num(0)
		case "Tf":
			if len(args) == 2 {
				name := args[0].Name()
				f, ok := fonts[name]
				if !ok {
					ff := p.Font(name)
					f = &ff
					fonts[name] = f
				}
				g.Tf, g.enc, g.Tfs = *f, f.Encoder(), num(1)
			}
		case "TD":
			g.Tl = -num(1)
			fallthrough
		case "Td":
			g.Tlm = tzMatrix{{1, 0, 0}, {0, 1, 0}, {num(0), num(1), 1}}.mul(g.Tlm)
			g.Tm = g.Tlm
		case "Tm":
			if len(args) == 6 {
				var m tzMatrix
				for i := 0; i < 6; i++ {
					m[i/2][i%2] = num(i)
				}
				m[2][2] = 1
				g.Tm, g.Tlm = m, m
			}
		case "T*":
			nextLine()
		case "\"":
			if len(args) == 3 {
				g.Tw, g.Tc = num(0), num(1)
				nextLine()
				show(args[2].RawString())
			}
		case "'":
			if len(args) == 1 {
				nextLine()
				show(args[0].RawString())
			}
		case "Tj":
			if len(args) == 1 {
				show(args[0].RawString())
			}
		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for i := 0; i < v.Len(); i++ {
					x := v.Index(i)
					if x.Kind() == pdf.String {
						show(x.RawString())
					} else {
						tx := -x.Float64() / 1000 * g.Tfs * g.Th
						g.Tm = tzMatrix{{1, 0, 0}, {0, 1, 0}, {tx, 0, 1}}.mul(g.Tm)
					}
				}
			}
		}
	})
	return text
}
