package helper

import (
	"os"
	osruntime "runtime"
	"time"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
)

func PrintVersion(logger logr.Logger, metricsAddr, probeAddr string) {

	logLevel, _ := os.LookupEnv("LOG_LEVEL")
	logFormatter, _ := os.LookupEnv("LOG_FORMATTER")

	logger.Info("Binary info ", "Go version", osruntime.Version())
	logger.Info("Binary info ", "OS", osruntime.GOOS, "Arch", osruntime.GOARCH)
	logger.Info("Address ", "Metrics", metricsAddr)
	logger.Info("Address ", "Probe", probeAddr)
	logger.Info("Log level ", logLevel)
	logger.Info("Log formatter ", logFormatter)
}

func GetWatchNamespaceFromEnv() (ns string, err error) {

	watchNamespaceEnvVar := "WATCH_NAMESPACES"
	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", errors.Errorf("%s must be set", watchNamespaceEnvVar)
	}

	return ns, nil
}

func GetKubeClientTimeoutFromEnv() (timeout time.Duration, err error) {
	kubeClientTimeoutEnvVar := "KUBE_CLIENT_TIMEOUT"
	t, found := os.LookupEnv(kubeClientTimeoutEnvVar)
	if !found {
		return 30 * time.Second, nil
	}

	timeout, err = time.ParseDuration(t)
	if err != nil {
		return 0, err
	}

	return timeout, nil
}
