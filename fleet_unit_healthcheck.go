package main

import (
	"errors"
	"flag"
	"github.com/Financial-Times/go-fthealth"
	"github.com/coreos/fleet/client"
	"github.com/coreos/fleet/schema"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/net/proxy"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

func main() {
	var (
		socksProxy    = flag.String("socks-proxy", "", "Use specified SOCKS proxy (e.g. localhost:2323)")
		fleetEndpoint = flag.String("fleetEndpoint", "", "Fleet API http endpoint: `http://host:port`")
	)

	flag.Parse()

	fleetAPIClient, err := newFleetApiClient(*fleetEndpoint, *socksProxy)
	if err != nil {
		panic(err)
	}
	handler := FleetUnitHealthHandler(fleetAPIClient, FleetUnitHealthChecker{})

	r := mux.NewRouter()
	r.HandleFunc("/", handler)
	r.HandleFunc("/__health", handler)

	err = http.ListenAndServe(":8080", handlers.LoggingHandler(os.Stdout, r))
	if err != nil {
		panic(err)
	}
}

func newFleetApiClient(fleetEndpoint string, socksProxy string) (client.API, error) {
	u, err := url.Parse(fleetEndpoint)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{}

	if socksProxy != "" {
		log.Printf("using proxy %s\n", socksProxy)
		netDialler := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialer, err := proxy.SOCKS5("tcp", socksProxy, nil, netDialler)
		if err != nil {
			log.Fatalf("error with proxy %s: %v\n", socksProxy, err)
		}
		httpClient.Transport = &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
	}

	return client.NewHTTPClient(httpClient, *u)
}

func FleetUnitHealthHandler(fleetAPIClient client.API, checker FleetUnitHealthChecker) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := []fthealth.Check{}
		unitStates, err := fleetAPIClient.UnitStates()
		if err != nil {
			panic(err)
		}
		for _, unitState := range unitStates {
			checks = append(checks, NewFleetUnitHealthCheck(*unitState, checker))
		}
		fthealth.HandlerParallel("Coco Fleet Unit Healthcheck", "Checks the health of all fleet units", checks...)(w, r)
	}
}

func NewFleetUnitHealthCheck(unitState schema.UnitState, checker FleetUnitHealthChecker) fthealth.Check {
	return fthealth.Check{
		Name:             unitState.Name + "_" + unitState.MachineID,
		Severity:         2,
		Checker:          func() error { return checker.Check(unitState) },
		TechnicalSummary: "This fleet unit is in a failed state.",
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		PanicGuide:       "TO-DO",
	}
}

type FleetUnitHealthChecker struct{}

func (f *FleetUnitHealthChecker) Check(unitState schema.UnitState) error {
	if "failed" == unitState.SystemdActiveState {
		return errors.New("Unit is in failed state.")
	}

	return nil
}
