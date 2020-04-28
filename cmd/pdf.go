package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

func (s *searchContext) getPdfStatus(pdfURL, pid string) (string, error) {
	if pdfURL == "" || pid == "" {
		return "", fmt.Errorf("pdf url or pid is missing")
	}

	url := fmt.Sprintf("%s/%s%s", pdfURL, pid, s.svc.config.Pdf.Endpoints.Status)

	req, reqErr := http.NewRequest("GET", url, nil)
	if reqErr != nil {
		s.log("[PDF] NewRequest() failed: %s", reqErr.Error())
		return "", fmt.Errorf("failed to create PDF status request")
	}

	start := time.Now()
	res, resErr := s.svc.pdf.client.Do(req)
	elapsedMS := int64(time.Since(start) / time.Millisecond)

	// external service failure logging

	if resErr != nil {
		status := http.StatusBadRequest
		errMsg := resErr.Error()
		if strings.Contains(errMsg, "Timeout") {
			status = http.StatusRequestTimeout
			errMsg = fmt.Sprintf("%s timed out", url)
		} else if strings.Contains(errMsg, "connection refused") {
			status = http.StatusServiceUnavailable
			errMsg = fmt.Sprintf("%s refused connection", url)
		}

		s.log("[PDF] client.Do() failed: %s", resErr.Error())
		s.log("ERROR: Failed response from %s %s - %d:%s. Elapsed Time: %d (ms)", req.Method, url, status, errMsg, elapsedMS)
		return "", fmt.Errorf("failed to receive PDF status response")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Errorf("unexpected status code %d", res.StatusCode)
		s.log("[PDF] unexpected status code %d", res.StatusCode)
		s.log("ERROR: Failed response from %s %s - %d:%s. Elapsed Time: %d (ms)", req.Method, url, res.StatusCode, errMsg, elapsedMS)
		return "", fmt.Errorf("received PDF status response code %d", res.StatusCode)
	}

	status, err := ioutil.ReadAll(res.Body)

	if err != nil {
		s.log("[PDF] error reading pdf status response (%s)", err.Error())
		return "", fmt.Errorf("error reading pdf status response")
	}

	// external service success logging

	s.log("Successful PDF response from %s %s. Elapsed Time: %d (ms)", req.Method, s.svc.solr.url, elapsedMS)

	return string(status), nil
}
