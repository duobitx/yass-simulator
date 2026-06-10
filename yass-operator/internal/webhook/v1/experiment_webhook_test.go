/*
Copyright 2025.

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

package v1

import (
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Experiment Webhook", func() {
	var (
		obj       *yassv1.Experiment
		oldObj    *yassv1.Experiment
		validator ExperimentCustomValidator
		defaulter ExperimentCustomDefaulter
	)

	BeforeEach(func() {
		obj = &yassv1.Experiment{}
		oldObj = &yassv1.Experiment{}
		validator = ExperimentCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = ExperimentCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		// TODO (user): Add any setup logic common to all tests
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating Experiment under Defaulting Webhook", func() {
		// TODO (user): Add logic for defaulting webhooks
		// Example:
		// It("Should apply defaults when a required field is empty", func() {
		//     By("simulating a scenario where defaults should be applied")
		//     obj.SomeFieldWithDefault = ""
		//     By("calling the Default method to apply defaults")
		//     defaulter.Default(ctx, obj)
		//     By("checking that the default values are set")
		//     Expect(obj.SomeFieldWithDefault).To(Equal("default_value"))
		// })
	})

	Context("single-experiment-per-namespace guard on ValidateCreate", func() {
		newExp := func(name, ns string, mutate func(*yassv1.Experiment)) *yassv1.Experiment {
			e := &yassv1.Experiment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
			if mutate != nil {
				mutate(e)
			}
			return e
		}
		withSiblings := func(objs ...client.Object) ExperimentCustomValidator {
			sch := runtime.NewScheme()
			Expect(yassv1.AddToScheme(sch)).To(Succeed())
			c := fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()
			return ExperimentCustomValidator{Client: c}
		}

		It("admits the first experiment in an empty namespace", func() {
			v := withSiblings()
			_, err := v.ValidateCreate(ctx, newExp("first", "ns1", nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects a second experiment while a sibling still has running resources", func() {
			running := newExp("running", "ns1", func(e *yassv1.Experiment) {
				e.Status.ExperimentState = yassv1.ExperimentStateOngoing
			})
			v := withSiblings(running)
			_, err := v.ValidateCreate(ctx, newExp("second", "ns1", nil))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("single experiment per namespace"))
		})

		It("admits when the sibling's resources have already been evicted", func() {
			evicted := newExp("old", "ns1", func(e *yassv1.Experiment) {
				e.Status.ExperimentState = yassv1.ExperimentStateSuccess
				e.Annotations = map[string]string{yassv1.ResourcesEvictedAnnotation: "true"}
			})
			v := withSiblings(evicted)
			_, err := v.ValidateCreate(ctx, newExp("fresh", "ns1", nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("admits when the sibling is being deleted", func() {
			now := metav1.Now()
			deleting := newExp("old", "ns1", func(e *yassv1.Experiment) {
				e.Status.ExperimentState = yassv1.ExperimentStateOngoing
				e.DeletionTimestamp = &now
				e.Finalizers = []string{"yass/test-finalizer"}
			})
			v := withSiblings(deleting)
			_, err := v.ValidateCreate(ctx, newExp("fresh", "ns1", nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("ignores running experiments in other namespaces", func() {
			elsewhere := newExp("running", "ns2", func(e *yassv1.Experiment) {
				e.Status.ExperimentState = yassv1.ExperimentStateOngoing
			})
			v := withSiblings(elsewhere)
			_, err := v.ValidateCreate(ctx, newExp("here", "ns1", nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("skips the check entirely when no client is configured", func() {
			v := ExperimentCustomValidator{}
			_, err := v.ValidateCreate(ctx, newExp("any", "ns1", nil))
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
