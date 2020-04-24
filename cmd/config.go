package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
)

const envPrefix = "VIRGO4_DIGITAL_CONTENT_WS"

type serviceConfigSolrParams struct {
	Qt      string   `json:"qt,omitempty"`
	DefType string   `json:"deftype,omitempty"`
	Fq      []string `json:"fq,omitempty"`
	Fl      []string `json:"fl,omitempty"`
}

type serviceConfigSolr struct {
	Host        string                  `json:"host,omitempty"`
	Core        string                  `json:"core,omitempty"`
	Handler     string                  `json:"handler,omitempty"`
	ConnTimeout string                  `json:"conn_timeout,omitempty"`
	ReadTimeout string                  `json:"read_timeout,omitempty"`
	Params      serviceConfigSolrParams `json:"params,omitempty"`
}

type serviceConfigField struct {
	Name     string `json:"name,omitempty"`
	Field    string `json:"field,omitempty"`
	Required bool   `json:"required,omitempty"`
	Custom   bool   `json:"custom,omitempty"` // custom handling by Name
	Array    bool   `json:"array,omitempty"`
}

type serviceConfig struct {
	Port   string               `json:"port,omitempty"`
	JWTKey string               `json:"jwt_key,omitempty"`
	Solr   serviceConfigSolr    `json:"solr,omitempty"`
	Fields []serviceConfigField `json:"fields,omitempty"`
}

func getSortedJSONEnvVars() []string {
	var keys []string

	for _, keyval := range os.Environ() {
		key := strings.Split(keyval, "=")[0]
		if strings.HasPrefix(key, envPrefix+"_JSON_") {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	return keys
}

func loadConfig() *serviceConfig {
	cfg := serviceConfig{}

	// json configs

	envs := getSortedJSONEnvVars()

	valid := true

	for _, env := range envs {
		log.Printf("[CONFIG] loading %s ...", env)
		if val := os.Getenv(env); val != "" {
			dec := json.NewDecoder(bytes.NewReader([]byte(val)))
			dec.DisallowUnknownFields()

			if err := dec.Decode(&cfg); err != nil {
				log.Printf("error decoding %s: %s", env, err.Error())
				valid = false
			}
		}
	}

	if valid == false {
		log.Printf("exiting due to json decode error(s) above")
		os.Exit(1)
	}

	// optional convenience override to simplify terraform config
	if host := os.Getenv(envPrefix + "_SOLR_HOST"); host != "" {
		cfg.Solr.Host = host
	}

	//bytes, err := json.MarshalIndent(cfg, "", "  ")
	bytes, err := json.Marshal(cfg)
	if err != nil {
		log.Printf("error encoding config json: %s", err.Error())
		os.Exit(1)
	}

	log.Printf("[CONFIG] composite json:\n%s", string(bytes))

	return &cfg
}
