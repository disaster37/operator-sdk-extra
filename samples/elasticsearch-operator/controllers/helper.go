package controllers

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"emperror.dev/errors"
	eshandler "github.com/disaster37/es-handler/v8"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	elastic "github.com/elastic/go-elasticsearch/v8"
	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetElasticsearchHandler(ctx context.Context, o client.Object, esRef elasticsearchapicrd.ElasticsearchRef, client client.Client, log *logrus.Entry) (esHandler eshandler.ElasticsearchHandler, err error) {
	// Retrieve secret or elasticsearch resource that store the connexion credentials
	var secretNS types.NamespacedName
	secretName := ""
	isManaged := false
	hosts := []string{}
	selfSignedCertificate := false

	secretName = esRef.ExternalElasticsearchRef.SecretRef.Name
	hosts = esRef.ExternalElasticsearchRef.Addresses

	secretNS = types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      secretName,
	}

	// Read settings to access on Elasticsearch api
	secret := &core.Secret{}

	if err = client.Get(ctx, secretNS, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			log.Warnf("Secret %s not yet exist, try later", secretName)
			return nil, nil
		}
		log.Errorf("Error when get resource: %s", err.Error())
		return nil, err
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{},
		ResponseHeaderTimeout: 10 * time.Second,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	cfg := elastic.Config{
		Transport: transport,
		Addresses: hosts,
	}

	if log.Logger.GetLevel() == logrus.DebugLevel {
		cfg.Logger = &elastictransport.JSONLogger{EnableRequestBody: true, EnableResponseBody: true, Output: log.Logger.Out}
	}

	if isManaged {
		cfg.Username = "elastic"
		cfg.Password = string(secret.Data["elastic"])
	} else {
		if len(secret.Data["username"]) == 0 || len(secret.Data["password"]) == 0 {
			return nil, errors.Errorf("The secret %s must contain key `username` and `password`", secret.Name)
		}
		cfg.Username = string(secret.Data["username"])
		cfg.Password = string(secret.Data["password"])
	}

	if selfSignedCertificate {
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	// Create Elasticsearch handler/client
	esHandler, err = eshandler.NewElasticsearchHandler(cfg, log)
	if err != nil {
		return nil, err
	}

	return esHandler, nil
}
