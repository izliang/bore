package middleware

type middlewareParam struct {
	panicStackNewLine bool
}

type MiddleOptionFunc func(*middlewareParam)

func WithPanicMiddleStackNewLine(newline bool) MiddleOptionFunc {
	return func(p *middlewareParam) {
		p.panicStackNewLine = newline
	}
}
