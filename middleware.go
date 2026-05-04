package command

import (
	"encoding/json"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

var (
	_ caddy.Module                = (*Middleware)(nil)
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddy.Validator             = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
)

func init() {
	caddy.RegisterModule(Middleware{})
}

// MiddlewareWriter implements an unbuffered response writer.
type MiddlewareWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (mw *MiddlewareWriter) Write(p []byte) (int, error) {
	n, err := mw.w.Write(p)
	if err == nil {
		mw.f.Flush()
	}
	return n, err
}

// Middleware implements an HTTP handler that runs shell command.
type Middleware struct {
	Cmd
}

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.exec",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error { return m.Cmd.provision(ctx, m) }

// Validate implements caddy.Validator
func (m Middleware) Validate() error { return m.Cmd.validate() }

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	// replace per-request placeholders
	argv := make([]string, len(m.Args))
	for index, argument := range m.Args {
		argv[index] = repl.ReplaceAll(argument, "")
	}

	if m.Foreground {
		w.Header().Add("Trailer", "Status")
		w.WriteHeader(http.StatusOK)

		if f, ok := w.(http.Flusher); ok {
			f.Flush()

			mw := &MiddlewareWriter{w, f}
			m.stdWriter = mw
			m.errWriter = mw
		} else {
			m.stdWriter = w
			m.errWriter = w
		}
	}

	err := m.run(argv)

	if m.PassThru {
		if err != nil {
			m.log.Error(err.Error())
		}

		return next.ServeHTTP(w, r)
	}

	if m.Foreground {
		if err != nil {
			w.Header().Add("Status", err.Error())
		} else {
			w.Header().Add("Status", "OK")
		}
		return err
	}

	var resp struct {
		Status string `json:"status,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	if err == nil {
		resp.Status = "success"
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		resp.Error = err.Error()
	}

	w.Header().Add("content-type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

// Cleanup implements caddy.Cleanup
// TODO: ensure all running processes are terminated.
func (m *Middleware) Cleanup() error {
	return nil
}
