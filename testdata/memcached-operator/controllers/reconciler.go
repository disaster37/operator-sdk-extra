package controllers

import (
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BaseReconciler struct {
	client   client.Client
	recorder record.EventRecorder
	logger   *logrus.Entry
}
