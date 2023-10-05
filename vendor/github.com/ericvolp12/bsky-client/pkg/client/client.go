package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/xrpc"
	"golang.org/x/time/rate"
)

// Client is a Bluesky API Client
type Client struct {
	PLCHost         string
	PDSHost         string
	ActorDID        syntax.DID
	AutoRefreshAuth bool

	xrpcc     *xrpc.Client
	clientMux *sync.RWMutex
	dir       identity.Directory
	rl        *rate.Limiter
}

// New creates a new Bluesky API Client
func New(pdsHost, plcHost string) *Client {
	base := identity.BaseDirectory{
		PLCURL: plcHost,
		HTTPClient: http.Client{
			Timeout: time.Second * 15,
		},
		Resolver: net.Resolver{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Second * 5}
				return d.DialContext(ctx, network, address)
			},
		},
		TryAuthoritativeDNS: true,
		// primary Bluesky PDS instance only supports HTTP resolution method
		SkipDNSDomainSuffixes: []string{".bsky.social"},
	}
	dir := identity.NewCacheDirectory(&base, 10000, time.Hour*24, time.Minute*2)

	return &Client{
		PDSHost: pdsHost,
		PLCHost: plcHost,
		xrpcc: &xrpc.Client{
			Host: pdsHost,
			Client: &http.Client{
				Timeout: 30 * time.Second,
			},
		},
		AutoRefreshAuth: true,
		dir:             &dir,
		rl:              rate.NewLimiter(rate.Limit(8), 1),
	}
}

// Login logs in the user with the given handle and appPassword
func (c *Client) Login(ctx context.Context, handle, appPassword string) error {
	h, err := syntax.ParseHandle(handle)
	if err != nil {
		return fmt.Errorf("failed to parse handle: %w", err)
	}

	// Figure out what PDS the handle is on
	ident, err := c.dir.LookupHandle(ctx, h)
	if err != nil {
		return fmt.Errorf("failed to lookup handle: %w", err)
	}

	if c.PDSHost != ident.PDSEndpoint() {
		slog.DebugContext(ctx, "handle is on a different PDS, switching client to that PDS", "pds", ident.PDSEndpoint())
		c.PDSHost = ident.PDSEndpoint()
	}
	c.xrpcc.Host = c.PDSHost

	if c.rl != nil {
		c.rl.Wait(ctx)
	}
	ses, err := comatproto.ServerCreateSession(ctx, c.xrpcc, &comatproto.ServerCreateSession_Input{
		Identifier: handle,
		Password:   appPassword,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	c.xrpcc.Auth = &xrpc.AuthInfo{
		Handle:     ses.Handle,
		Did:        ses.Did,
		RefreshJwt: ses.RefreshJwt,
		AccessJwt:  ses.AccessJwt,
	}

	did, err := syntax.ParseDID(ses.Did)
	if err != nil {
		return fmt.Errorf("failed to parse did: %w", err)
	}

	c.ActorDID = did

	if c.AutoRefreshAuth {
		// Start a goroutine to refresh the auth token every 10 minutes
		go func() {
			t := time.NewTicker(10 * time.Minute)

			for {
				ctx := context.Background()
				select {
				case <-t.C:
					slog.DebugContext(ctx, "refreshing auth token")
					err := c.RefreshAuth(ctx)
					if err != nil {
						slog.ErrorContext(ctx, "failed to refresh auth token", "error", err)
					}
				}
			}
		}()
	}

	return nil
}

// RefreshAuth refreshes the auth token for the client
func (c *Client) RefreshAuth(ctx context.Context) error {
	c.clientMux.Lock()
	defer c.clientMux.Unlock()

	c.xrpcc.Auth.AccessJwt = c.xrpcc.Auth.RefreshJwt

	if c.rl != nil {
		c.rl.Wait(ctx)
	}
	refreshedSession, err := comatproto.ServerRefreshSession(ctx, c.xrpcc)
	if err != nil {
		return fmt.Errorf("failed to refresh session: %w", err)
	}

	c.xrpcc.Auth = &xrpc.AuthInfo{
		Handle:     refreshedSession.Handle,
		Did:        refreshedSession.Did,
		RefreshJwt: refreshedSession.RefreshJwt,
		AccessJwt:  refreshedSession.AccessJwt,
	}

	return nil
}
