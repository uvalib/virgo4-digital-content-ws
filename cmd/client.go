package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uvalib/virgo4-jwt/v4jwt"
)

type clientOpts struct {
	debug   bool // controls whether debug info is added to response json
	verbose bool // controls whether verbose Solr requests/responses are logged
}

type clientContext struct {
	reqID  string          // internally generated
	start  time.Time       // internally set
	opts   clientOpts      // options set by client
	claims *v4jwt.V4Claims // information about this user
	nolog  bool            // internally set
	ginCtx *gin.Context    // gin context
}

func boolOptionWithFallback(opt string, fallback bool) bool {
	var err error
	var val bool

	if val, err = strconv.ParseBool(opt); err != nil {
		val = fallback
	}

	return val
}

func (c *clientContext) init(p *serviceContext, ctx *gin.Context) {
	c.ginCtx = ctx

	c.start = time.Now()
	c.reqID = fmt.Sprintf("%08x", p.randomSource.Uint32())

	// get claims, if any
	if val, ok := ctx.Get("claims"); ok == true {
		c.claims = val.(*v4jwt.V4Claims)
	}

	c.opts.debug = boolOptionWithFallback(ctx.Query("debug"), false)
	c.opts.verbose = boolOptionWithFallback(ctx.Query("verbose"), false)
}

func (c *clientContext) logRequest() {
	query := ""
	if c.ginCtx.Request.URL.RawQuery != "" {
		query = fmt.Sprintf("?%s", c.ginCtx.Request.URL.RawQuery)
	}

	claimsStr := ""
	if c.claims != nil {
		claimsStr = fmt.Sprintf("  [%s; %s; %s; %v]", c.claims.UserID, c.claims.Role, c.claims.AuthMethod, c.claims.IsUVA)
	}

	c.log("[REQUEST] %s %s%s  %s", c.ginCtx.Request.Method, c.ginCtx.Request.URL.Path, query, claimsStr)
}

func (c *clientContext) logResponse(resp searchResponse) {
	msg := fmt.Sprintf("[RESPONSE] status: %d", resp.status)

	if resp.err != nil {
		msg = msg + fmt.Sprintf(", error: %s", resp.err.Error())
	}

	c.log(msg)
}

func (c *clientContext) printf(prefix, format string, args ...interface{}) {
	str := fmt.Sprintf(format, args...)

	if prefix != "" {
		str = strings.Join([]string{prefix, str}, " ")
	}

	log.Printf("[%s] %s", c.reqID, str)
}

func (c *clientContext) log(format string, args ...interface{}) {
	if c.nolog == true {
		return
	}

	c.printf("", format, args...)
}

func (c *clientContext) err(format string, args ...interface{}) {
	c.printf("ERROR:", format, args...)
}
