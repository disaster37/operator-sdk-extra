package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func (h *Memcached) GetPhaseName() PhaseName
func (h *Memcached) SetPhaseName(name PhaseName)
func (h *Memcached) GetConditions() []metav1.Condition
func (h *Memcached) SetConditions(conditions []metav1.Condition)
func (h *Memcached) IsOnError() bool
func (h *Memcached) SetIsOnError(isError bool)
func (h *Memcached) LastErrorMessage() string
func (h *Memcached) SetLastErrorMessage(message string)
