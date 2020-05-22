package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// git commit used for this build; supplied at compile time
var gitCommit string

type serviceVersion struct {
	BuildVersion string `json:"build,omitempty"`
	GoVersion    string `json:"go_version,omitempty"`
	GitCommit    string `json:"git_commit,omitempty"`
}

type serviceSolrContext struct {
	client *http.Client
	url    string
}

type serviceSolr struct {
	service     serviceSolrContext
	healthcheck serviceSolrContext
}

type servicePdf struct {
	client *http.Client
}

type serviceContext struct {
	randomSource *rand.Rand
	config       *serviceConfig
	version      serviceVersion
	solr         serviceSolr
	pdf          servicePdf
}

type stringValidator struct {
	values  []string
	invalid bool
}

func (v *stringValidator) addValue(value string) {
	if value != "" {
		v.values = append(v.values, value)
	}
}

func (v *stringValidator) requireValue(value string, label string) {
	if value == "" {
		log.Printf("[VALIDATE] missing %s", label)
		v.invalid = true
		return
	}

	v.addValue(value)
}

func (v *stringValidator) Values() []string {
	return v.values
}

func (v *stringValidator) Invalid() bool {
	return v.invalid
}

func (p *serviceContext) initVersion() {
	buildVersion := "unknown"
	files, _ := filepath.Glob("buildtag.*")
	if len(files) == 1 {
		buildVersion = strings.Replace(files[0], "buildtag.", "", 1)
	}

	p.version = serviceVersion{
		BuildVersion: buildVersion,
		GoVersion:    fmt.Sprintf("%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
		GitCommit:    gitCommit,
	}

	log.Printf("[SERVICE] version.BuildVersion = [%s]", p.version.BuildVersion)
	log.Printf("[SERVICE] version.GoVersion    = [%s]", p.version.GoVersion)
	log.Printf("[SERVICE] version.GitCommit    = [%s]", p.version.GitCommit)
}

func httpClientWithTimeouts(conn, read string) *http.Client {
	connTimeout := integerWithMinimum(conn, 1)
	readTimeout := integerWithMinimum(read, 1)

	client := &http.Client{
		Timeout: time.Duration(readTimeout) * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(connTimeout) * time.Second,
				KeepAlive: 60 * time.Second,
			}).DialContext,
			MaxIdleConns:        100, // we are hitting one solr host, so
			MaxIdleConnsPerHost: 100, // these two values can be the same
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return client
}

func (p *serviceContext) initSolr() {
	// client setup

	serviceCtx := serviceSolrContext{
		url:    fmt.Sprintf("%s/%s/%s", p.config.Solr.Host, p.config.Solr.Core, p.config.Solr.Clients.Service.Endpoint),
		client: httpClientWithTimeouts(p.config.Solr.Clients.Service.ConnTimeout, p.config.Solr.Clients.Service.ReadTimeout),
	}

	healthCtx := serviceSolrContext{
		url:    fmt.Sprintf("%s/%s/%s", p.config.Solr.Host, p.config.Solr.Core, p.config.Solr.Clients.HealthCheck.Endpoint),
		client: httpClientWithTimeouts(p.config.Solr.Clients.HealthCheck.ConnTimeout, p.config.Solr.Clients.HealthCheck.ReadTimeout),
	}

	solr := serviceSolr{
		service:     serviceCtx,
		healthcheck: healthCtx,
	}

	p.solr = solr

	log.Printf("[SERVICE] solr service url     = [%s]", serviceCtx.url)
	log.Printf("[SERVICE] solr healthcheck url = [%s]", healthCtx.url)
}

func (p *serviceContext) initPdf() {
	// client setup

	p.pdf = servicePdf{
		client: httpClientWithTimeouts(p.config.Pdf.ConnTimeout, p.config.Pdf.ReadTimeout),
	}
}

func (p *serviceContext) validateConfig() {
	// ensure the existence and validity of required variables/solr fields

	invalid := false

	var solrFields stringValidator
	var miscValues stringValidator

	miscValues.requireValue(p.config.Solr.Host, "solr host")
	miscValues.requireValue(p.config.Solr.Core, "solr core")
	miscValues.requireValue(p.config.Solr.Clients.Service.Endpoint, "solr service endpoint")
	miscValues.requireValue(p.config.Solr.Clients.HealthCheck.Endpoint, "solr healthcheck endpoint")
	miscValues.requireValue(p.config.Solr.Params.Qt, "solr param qt")
	miscValues.requireValue(p.config.Solr.Params.DefType, "solr param deftype")

	for _, field := range p.config.Fields.Item {
		miscValues.requireValue(field.Name, "item field name")
		solrFields.requireValue(field.Field, "item solr field")
	}

	for _, field := range p.config.Fields.Parts.Indexed {
		miscValues.requireValue(field.Name, "indexed parts field name")
		solrFields.requireValue(field.Field, "indexed parts solr field")
	}

	for _, field := range p.config.Fields.Parts.Custom {
		miscValues.requireValue(field.Name, "custom parts field name")

		switch field.Name {
		case "iiif_manifest_url":
			solrFields.requireValue(field.Field, fmt.Sprintf("custom parts %s solr field", field.Name))

			if field.CustomInfo == nil {
				log.Printf("[VALIDATE] missing custom parts %s custom info section", field.Name)
				invalid = true
				continue
			}

			if field.CustomInfo.IIIFManifestURL == nil {
				log.Printf("[VALIDATE] missing custom parts %s custom info %s section", field.Name, field.Name)
				invalid = true
				continue
			}

			miscValues.requireValue(field.CustomInfo.IIIFManifestURL.URLPrefix, fmt.Sprintf("missing custom parts %s custom info %s section url prefix", field.Name, field.Name))

		case "pdf":
			solrFields.requireValue(field.Field, fmt.Sprintf("custom parts %s solr field", field.Name))

		default:
			log.Printf("[VALIDATE] unhandled custom field: [%s]", field.Name)
			invalid = true
		}
	}

	// validate solr fields can actually be found in a solr document

	doc := solrDocument{}

	for _, tag := range solrFields.Values() {
		if val := doc.getFieldByTag(tag); val == nil {
			log.Printf("[VALIDATE] field not found in Solr document struct tags: [%s]", tag)
			invalid = true
		}
	}

	// check if anything went wrong anywhere

	if invalid || solrFields.Invalid() || miscValues.Invalid() {
		log.Printf("[VALIDATE] exiting due to error(s) above")
		os.Exit(1)
	}
}

func initializeService(cfg *serviceConfig) *serviceContext {
	p := serviceContext{}

	p.config = cfg
	p.randomSource = rand.New(rand.NewSource(time.Now().UnixNano()))

	p.initVersion()
	p.initSolr()
	p.initPdf()

	p.validateConfig()

	return &p
}
