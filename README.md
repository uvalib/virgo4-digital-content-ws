# Virgo4 Digital Content Web Service

This is a web service to retrieve digital content from Solr.

* GET /version : returns build version
* GET /healthcheck : returns health check information
* GET /metrics : returns Prometheus metrics
* GET /api/resource/{id} : returns digital content for a single record in Solr

All endpoints under /api require authentication.

### System Requirements

* GO version 1.12.0 or greater
