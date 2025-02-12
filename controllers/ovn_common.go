/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
)

// fields to index to reconcile when changed
const (
	tlsField                = ".spec.tls.secretName"
	caBundleSecretNameField = ".spec.tls.caBundleSecretName"
	topologyField           = ".spec.topologyRef.Name"
)

var (
	allWatchFields = []string{
		caBundleSecretNameField,
		tlsField,
		topologyField,
	}
)

// ensureOVNTopology - when a Topology CR is referenced, remove the
// finalizer from a previous referenced Topology (if any), and retrieve the
// newly referenced topology object
func ensureOVNTopology(
	ctx context.Context,
	helper *helper.Helper,
	tpRef *topologyv1.TopoRef,
	lastAppliedTopology *topologyv1.TopoRef,
	finalizer string,
	selector string,
) (*topologyv1.Topology, error) {

	var podTopology *topologyv1.Topology
	var err error

	// Remove (if present) the finalizer from a previously referenced topology
	//
	// 1. a topology reference is removed (tpRef == nil) from the Service Component
	//    subCR and the finalizer should be deleted from the last applied topology
	//    (lastAppliedTopology != "")
	// 2. a topology reference is updated in the Service Component CR (tpRef != nil)
	//    and the finalizer should be removed from the previously
	//    referenced topology (tpRef.Name != lastAppliedTopology.Name)
	if (tpRef == nil && lastAppliedTopology.Name != "") ||
		(tpRef != nil && tpRef.Name != lastAppliedTopology.Name) {
		_, err = topologyv1.EnsureDeletedTopologyRef(
			ctx,
			helper,
			lastAppliedTopology,
			finalizer,
		)
		if err != nil {
			return nil, err
		}
	}
	// TopologyRef is passed as input, get the Topology object
	if tpRef != nil {
		// no Namespace is provided, default to instance.Namespace
		if tpRef.Namespace == "" {
			tpRef.Namespace = helper.GetBeforeObject().GetNamespace()
		}
		// Build a defaultLabelSelector
		defaultLabelSelector := labels.GetAppLabelSelector(selector)

		// Retrieve the referenced Topology
		podTopology, _, err = topologyv1.EnsureTopologyRef(
			ctx,
			helper,
			tpRef,
			finalizer,
			&defaultLabelSelector,
		)
		if err != nil {
			return nil, err
		}
	}
	return podTopology, nil
}
