package test

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type SchemeRegistration func(scheme *runtime.Scheme)

type ControllerSetupFunc func(mgr ctrl.Manager, k8sClient client.Client, cfg *rest.Config)

type EnvTestSuiteConfig struct {
	CRDDirectoryPaths     []string
	SchemeRegistrations   []SchemeRegistration
	ControllerSetupFuncs  []ControllerSetupFunc
	MetricsBindAddress    string
	ErrorIfCRDPathMissing bool
}

type EnvTestSuite struct {
	suite.Suite
	K8sClient client.Client
	Cfg       *rest.Config
	Manager   ctrl.Manager

	Config  EnvTestSuiteConfig
	testEnv *envtest.Environment
}

func (s *EnvTestSuite) SetupSuite() {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableQuote: true,
	})

	s.testEnv = &envtest.Environment{
		CRDDirectoryPaths:        s.Config.CRDDirectoryPaths,
		ErrorIfCRDPathMissing:    s.Config.ErrorIfCRDPathMissing,
		ControlPlaneStopTimeout:  120 * time.Second,
		ControlPlaneStartTimeout: 120 * time.Second,
	}
	cfg, err := s.testEnv.Start()
	if err != nil {
		panic(err)
	}
	s.Cfg = cfg

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	for _, reg := range s.Config.SchemeRegistrations {
		reg(scheme)
	}

	metricsAddr := s.Config.MetricsBindAddress
	if metricsAddr == "" {
		metricsAddr = "0"
	}

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: metricsAddr},
	})
	if err != nil {
		panic(err)
	}
	s.Manager = k8sManager
	s.K8sClient = k8sManager.GetClient()

	for _, setupFunc := range s.Config.ControllerSetupFuncs {
		setupFunc(k8sManager, s.K8sClient, cfg)
	}

	go func() {
		if err := k8sManager.Start(ctrl.SetupSignalHandler()); err != nil {
			panic(err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !k8sManager.GetCache().WaitForCacheSync(ctx) {
		panic("Failed to sync cache")
	}
}

func (s *EnvTestSuite) TearDownSuite() {
	err := s.testEnv.Stop()
	if err != nil {
		panic(err)
	}
}
