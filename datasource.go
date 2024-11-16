package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/patrulek/trojanbotproxy/config"
)

type httpDataSource struct {
	*httpClient
	interval  time.Duration
	tokenPath string

	mu     sync.RWMutex
	tokens []string

	doneC chan struct{}
}

func NewHttpDataSource(cfg config.DataSource) (*httpDataSource, error) {
	if cfg.Host == "" || cfg.Port == 0 || cfg.Method == "" || cfg.Auth.Context == "" || cfg.Auth.Name == "" || cfg.Auth.Value == "" || cfg.Interval == "" || len(cfg.Params) == 0 || cfg.TokenPath == "" {
		return nil, fmt.Errorf("invalid config")
	}

	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse interval: %w", err)
	}

	return &httpDataSource{
		httpClient: newHttpClient(cfg),
		interval:   interval,
		tokenPath:  cfg.TokenPath,
		doneC:      make(chan struct{}),
	}, nil
}

type httpClient struct {
	*http.Client
	host    string
	port    int
	method  string
	params  map[string]string
	headers map[string]string
}

func newHttpClient(cfg config.DataSource) *httpClient {
	headers := make(map[string]string)
	headers["Accept"] = "application/json"
	headers["Content-Type"] = "application/json"
	if cfg.Auth.Context == "header" {
		headers[cfg.Auth.Name] = cfg.Auth.Value
	}

	return &httpClient{
		Client:  &http.Client{},
		headers: headers,
		host:    cfg.Host,
		port:    cfg.Port,
		method:  cfg.Method,
		params:  cfg.Params,
	}
}

func (d *httpDataSource) Start(ctx context.Context) error {
	slog.Info("starting http data source")

	go func() {
		defer close(d.doneC)

		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				uri := d.combineURI()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
				if err != nil {
					panic(err)
				}

				for k, v := range d.headers {
					req.Header.Add(k, v)
				}

				resp, err := d.Do(req)
				if err != nil {
					slog.Error("failed to do request", "error", err)
					continue
				}

				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					slog.Error("unexpected status code", "status", resp.StatusCode)
					continue
				}

				var dsResp DataSourceResponse
				if err := json.NewDecoder(resp.Body).Decode(&dsResp); err != nil {
					slog.Error("failed to decode response", "error", err)
					continue
				}

				if len(dsResp) == 0 {
					continue
				}

				d.mu.Lock()
				for _, dsRespItem := range dsResp {
					tokenObject, ok := dsRespItem[d.tokenPath]
					if !ok {
						slog.Warn("token not found in response item", "item", dsRespItem)
						continue
					}

					token, ok := tokenObject.(string)
					if !ok {
						slog.Warn("token is not a string", "token", tokenObject)
						continue
					}

					d.tokens = append(d.tokens, token)
				}
				d.mu.Unlock()
			}
		}
	}()

	return nil
}

func (d *httpDataSource) Stop(ctx context.Context) {
	slog.Info("stopping http data source")

	select {
	case <-d.doneC:
		return
	case <-ctx.Done():
		slog.Error("http data source stopped unexpectedly", "error", ctx.Err())
	}
}

func (d *httpDataSource) Retrieve(ctx context.Context) ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tokens := d.tokens
	d.tokens = nil

	return tokens, nil
}

func (d *httpDataSource) combineURI() string {
	uri := fmt.Sprintf("%s:%d/%s", d.host, d.port, d.method)
	if len(d.params) > 0 {
		uri += "?"
		for k, v := range d.params {
			uri += fmt.Sprintf("%s=%s&", k, v)
		}
	}

	return uri[:len(uri)-1]
}

type DataSourceResponse []DataSourceResponseItem // List of objects

type DataSourceResponseItem map[string]any
