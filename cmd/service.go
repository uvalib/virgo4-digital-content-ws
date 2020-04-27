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

type serviceSolr struct {
	client *http.Client
	url    string
}

type serviceContext struct {
	randomSource *rand.Rand
	config       *serviceConfig
	version      serviceVersion
	solr         serviceSolr
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

func (p *serviceContext) initSolr() {
	// client setup

	connTimeout := timeoutWithMinimum(p.config.Solr.ConnTimeout, 5)
	readTimeout := timeoutWithMinimum(p.config.Solr.ReadTimeout, 5)

	solrClient := &http.Client{
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

	p.solr = serviceSolr{
		url:    fmt.Sprintf("%s/%s/%s", p.config.Solr.Host, p.config.Solr.Core, p.config.Solr.Handler),
		client: solrClient,
	}

	log.Printf("[SERVICE] solr.url             = [%s]", p.solr.url)
}

func (p *serviceContext) validateConfig() {
	// ensure the existence and validity of required variables/solr fields

	invalid := false

	var solrFields stringValidator
	var miscValues stringValidator

	miscValues.requireValue(p.config.Solr.Host, "solr host")
	miscValues.requireValue(p.config.Solr.Core, "solr core")
	miscValues.requireValue(p.config.Solr.Handler, "solr handler")
	miscValues.requireValue(p.config.Solr.Params.Qt, "solr param qt")
	miscValues.requireValue(p.config.Solr.Params.DefType, "solr param deftype")

	for _, field := range p.config.Fields {
		miscValues.requireValue(field.Name, "field name")

		if field.Custom == true {
			switch field.Name {
			case "iiif_manifest_url":
				solrFields.addValue(field.Field)

			case "pdf_status":
				solrFields.addValue(field.Field)

			case "pdf_url":
				solrFields.addValue(field.Field)

			default:
				log.Printf("[VALIDATE] unhandled custom field: [%s]", field.Name)
				invalid = true
			}
		} else {
			solrFields.requireValue(field.Field, "solr field")
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

	p.validateConfig()

	return &p
}
