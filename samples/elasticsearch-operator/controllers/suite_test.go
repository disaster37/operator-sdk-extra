package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/es-handler/v8/mocks"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/mock"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
	//+kubebuilder:scaffold:imports
)

var testEnv *envtest.Environment

type ElasticsearchapiControllerTestSuite struct {
	suite.Suite
	k8sClient                client.Client
	cfg                      *rest.Config
	mockCtrl                 *gomock.Controller
	mockElasticsearchHandler *mocks.MockElasticsearchHandler
}

func TestElasticsearchapiControllerSuite(t *testing.T) {
	suite.Run(t, new(ElasticsearchapiControllerTestSuite))
}

func (t *ElasticsearchapiControllerTestSuite) SetupSuite() {
	t.mockCtrl = gomock.NewController(t.T())
	t.mockElasticsearchHandler = mocks.NewMockElasticsearchHandler(t.mockCtrl)

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableQuote: true,
	})

	// Setup testenv
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing:    true,
		ControlPlaneStopTimeout:  120 * time.Second,
		ControlPlaneStartTimeout: 120 * time.Second,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}
	t.cfg = cfg

	// Add CRD sheme
	err = scheme.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
	err = elasticsearchapicrd.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	// Init k8smanager and k8sclient
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		panic(err)
	}
	k8sClient := k8sManager.GetClient()
	t.k8sClient = k8sClient

	// Init controllers

	roleReconciler := NewRoleReconciler(
		k8sClient,
		logrus.NewEntry(logrus.StandardLogger()),
		k8sManager.GetEventRecorderFor("elasticsearch-role-controller"),
	)
	roleReconciler.(*RoleReconciler).reconcilerAction = mock.NewMockRemoteReconcilerAction[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler](
		roleReconciler.(*RoleReconciler).reconcilerAction,
		func(ctx context.Context, req reconcile.Request, o object.RemoteObject) (handler controller.RemoteExternalReconciler[*elasticsearchapicrd.Role, *eshandler.XPackSecurityRole, eshandler.ElasticsearchHandler], res reconcile.Result, err error) {
			return newRoleApiClient(t.mockElasticsearchHandler), res, nil
		},
	)
	if err = roleReconciler.SetupWithManager(k8sManager); err != nil {
		panic(err)
	}

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		if err != nil {
			panic(err)
		}
	}()
}

func (t *ElasticsearchapiControllerTestSuite) TearDownSuite() {
	// Teardown the test environment once controller is fnished.
	// Otherwise from Kubernetes 1.21+, teardon timeouts waiting on
	// kube-apiserver to return
	err := testEnv.Stop()
	if err != nil {
		panic(err)
	}
}

func (t *ElasticsearchapiControllerTestSuite) BeforeTest(suiteName, testName string) {
}

func (t *ElasticsearchapiControllerTestSuite) AfterTest(suiteName, testName string) {
	defer t.mockCtrl.Finish()
}
