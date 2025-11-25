// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package pdusession

import (
	"github.com/5GC-DEV/util-cdac/http2_util"
	utilLogger "github.com/5GC-DEV/util-cdac/logger"
	"github.com/omec-project/smf/logger"
	"github.com/omec-project/smf/pfcp"
	"github.com/omec-project/smf/pfcp/udp"
)

/*func DummyServer() {
	router := utilLogger.NewGinWithZap(logger.GinLog)

	AddService(router)

	go udp.Run(pfcp.Dispatch)

	smfKeyLogPath := "/opt/sslkey.log"
	smfPemPath := "/var/run/certs/tls.pem"
	smfkeyPath := "/var/run/certs/tls.key"

	var server *http.Server
	if srv, err := http2_util.NewServer(":29502", smfKeyLogPath, router); err != nil {
	} else {
		server = srv
	}

	if err := server.ListenAndServeTLS(smfPemPath, smfkeyPath); err != nil {
		logger.PduSessLog.Fatalln(err)
	}
} */

func DummyServer() {
	router := utilLogger.NewGinWithZap(logger.GinLog)

	AddService(router)

	go udp.Run(pfcp.Dispatch)

	smfKeyLogPath := "/tmp/sslkey.log"
	smfPemPath := "/var/run/certs/tls.pem"
	smfKeyPath := "/var/run/certs/tls.key"
	bindAddr := ":29502"

	// New server object according to updated NewServer()
	server, err := http2_util.NewServer(
		bindAddr,
		smfKeyLogPath,
		smfPemPath,
		smfKeyPath,
		router,
	)
	if err != nil {
		logger.PduSessLog.Fatalf("Failed to create server: %+v", err)
	}

	// No need to pass certs again here → TLS already configured in NewServer()
	if err := server.ListenAndServeTLS("", ""); err != nil {
		logger.PduSessLog.Fatalln(err)
	}
}
