package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uvalib/virgo4-jwt/v4jwt"
)

func (p *serviceContext) itemHandler(c *gin.Context) {
	cl := clientContext{}
	cl.init(p, c)

	s := searchContext{}
	s.init(p, &cl)

	s.id = c.Param("id")

	cl.logRequest()
	resp := s.handleItemRequest()
	cl.logResponse(resp)

	if resp.err != nil {
		c.String(resp.status, resp.err.Error())
		return
	}

	c.JSON(resp.status, resp.data)
}

func (p *serviceContext) ignoreHandler(c *gin.Context) {
}

func (p *serviceContext) versionHandler(c *gin.Context) {
	cl := clientContext{}
	cl.init(p, c)

	c.JSON(http.StatusOK, p.version)
}

func (p *serviceContext) healthCheckHandler(c *gin.Context) {
	cl := clientContext{}
	cl.init(p, c)

	s := searchContext{}
	s.init(p, &cl)

	if s.client.opts.verbose == false {
		s.client.nolog = true
	}

	// fill out Solr query directly, bypassing query syntax parser
	s.id = "pingtest"

	cl.logRequest()
	ping := s.handlePingRequest()
	cl.logResponse(ping)

	// build response

	type hcResp struct {
		Healthy bool   `json:"healthy"`
		Message string `json:"message,omitempty"`
	}

	hcSolr := hcResp{Healthy: true}
	if ping.err != nil {
		hcSolr = hcResp{Healthy: false, Message: ping.err.Error()}
	}

	hcMap := make(map[string]hcResp)

	hcMap["solr"] = hcSolr

	c.JSON(ping.status, hcMap)
}

func getBearerToken(authorization string) (string, error) {
	components := strings.Split(strings.Join(strings.Fields(authorization), " "), " ")

	// must have two components, the first of which is "Bearer", and the second a non-empty token
	if len(components) != 2 || components[0] != "Bearer" || components[1] == "" {
		return "", fmt.Errorf("invalid Authorization header: [%s]", authorization)
	}

	token := components[1]

	if token == "undefined" {
		return "", errors.New("bearer token is undefined")
	}

	return token, nil
}

func (p *serviceContext) authenticateHandler(c *gin.Context) {
	token, err := getBearerToken(c.GetHeader("Authorization"))
	if err != nil {
		log.Printf("Authentication failed: [%s]", err.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	claims, err := v4jwt.Validate(token, p.config.JWTKey)

	if err != nil {
		log.Printf("JWT signature for %s is invalid: %s", token, err.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Set("claims", claims)
}
