/*
Copyright The CloudNativePG Contributors

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

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	discoveryFake "k8s.io/client-go/discovery/fake"
	fakeClient "k8s.io/client-go/kubernetes/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("Kubernetes minor version detection",
	func(info *version.Info, detectedMinorVersion int, shouldSucceed bool) {
		result, err := extractK8sMinorVersion(info)
		Expect(result).To(Equal(detectedMinorVersion))
		Expect(err == nil).To(Equal(shouldSucceed))
	},
	Entry("When minor version is an integer", &version.Info{Minor: "25"}, 25, true),
	Entry("When minor version indicate backported patches", &version.Info{Minor: "21+"}, 21, true),
	Entry("When minor version is wrong", &version.Info{Minor: "c3p0"}, 0, false),
)

var _ = Describe("Set and unset Seccomp support", func() {
	It("should have seccomp support", func() {
		SetSeccompSupport(true)
		Expect(HaveSeccompSupport()).To(BeTrue())
	})

	It("should not have seccomp support", func() {
		SetSeccompSupport(false)
		Expect(HaveSeccompSupport()).To(BeFalse())
	})
})

var _ = Describe("Detect Seccomp support depending on", func() {
	client := fakeClient.NewSimpleClientset()
	fakeDiscovery := client.Discovery().(*discoveryFake.FakeDiscovery)

	It("version 1.22 not supported", func() {
		fakeDiscovery.FakedServerVersion = &version.Info{
			Major: "1",
			Minor: "22",
		}

		err := DetectSeccompSupport(client.Discovery())
		Expect(err).ToNot(HaveOccurred())
		Expect(HaveSeccompSupport()).To(BeFalse())
	})

	It("version 1.26 supported", func() {
		fakeDiscovery.FakedServerVersion = &version.Info{
			Major: "1",
			Minor: "26",
		}

		err := DetectSeccompSupport(client.Discovery())
		Expect(err).ToNot(HaveOccurred())
		Expect(HaveSeccompSupport()).To(BeTrue())
	})
})

var _ = Describe("Detect resources properly when", func() {
	client := fakeClient.NewSimpleClientset()
	fakeDiscovery := client.Discovery().(*discoveryFake.FakeDiscovery)

	It("should not detect PodMonitor resource", func() {
		exists, err := PodMonitorExist(client.Discovery())
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse())
	})

	It("should detect PodMonitor resource", func() {
		resources := []*metav1.APIResourceList{
			{
				GroupVersion: "monitoring.coreos.com/v1",
				APIResources: []metav1.APIResource{
					{
						Name: "podmonitors",
					},
				},
			},
		}
		fakeDiscovery.Resources = resources
		exists, err := PodMonitorExist(client.Discovery())
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("should not detect SecurityContextConstraints", func() {
		err := DetectSecurityContextConstraints(client.Discovery())
		Expect(err).ToNot(HaveOccurred())

		Expect(HaveSecurityContextConstraints()).To(BeFalse())
	})

	It("should detect SecurityContextConstraints resource", func() {
		resources := []*metav1.APIResourceList{
			{
				GroupVersion: "security.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{
						Name: "securitycontextconstraints",
					},
				},
			},
		}
		fakeDiscovery.Resources = resources
		err := DetectSecurityContextConstraints(client.Discovery())
		Expect(err).ToNot(HaveOccurred())

		Expect(HaveSecurityContextConstraints()).To(BeTrue())
	})

	It("should not detect VolumeSnapshots", func() {
		err := DetectVolumeSnapshotExist(client.Discovery())
		Expect(err).ToNot(HaveOccurred())

		Expect(HaveVolumeSnapshot()).To(BeFalse())
	})

	It("should detect VolumeSnapshots resource", func() {
		resources := []*metav1.APIResourceList{
			{
				GroupVersion: "snapshot.storage.k8s.io/v1",
				APIResources: []metav1.APIResource{
					{
						Name: "volumesnapshots",
					},
				},
			},
		}
		fakeDiscovery.Resources = resources
		err := DetectVolumeSnapshotExist(client.Discovery())
		Expect(err).ToNot(HaveOccurred())

		Expect(HaveVolumeSnapshot()).To(BeTrue())
	})
})

var _ = Describe("AvailableArchitecture", func() {
	var (
		mockHashCalculator func(_ string) (hash string, err error)
		arch               *AvailableArchitecture
	)

	BeforeEach(func() {
		mockHashCalculator = func(_ string) (hash string, err error) {
			return "mockedHash", nil
		}
		arch = newAvailableArchitecture("amd64", filepath.Join("bin", "manager_amd64"))
		arch.hashCalculator = mockHashCalculator
	})

	Describe("GetHash", func() {
		Context("when hash is not calculated yet", func() {
			It("should calculate the hash", func() {
				hash := arch.GetHash()
				Expect(hash).To(Equal("mockedHash"))
			})
		})

		Context("when hash is already calculated", func() {
			BeforeEach(func() {
				arch.hash = "precalculatedHash"
			})

			It("should return the precalculated hash", func() {
				hash := arch.GetHash()
				Expect(hash).To(Equal("precalculatedHash"))
			})
		})
	})

	Describe("calculateHash", func() {
		Context("when hash is not calculated yet", func() {
			It("should calculate the hash", func() {
				arch.calculateHash()
				Expect(arch.hash).To(Equal("mockedHash"))
			})
		})

		Context("when hash is already calculated", func() {
			BeforeEach(func() {
				mockHashCalculator = func(_ string) (hash string, err error) {
					return "should-not-return-this", nil
				}
				arch.hash = "precalculatedHash"
			})

			It("should not recalculate the hash on subsequent calls", func() {
				arch.calculateHash()
				hash1 := arch.hash

				arch.hashCalculator = mockHashCalculator

				arch.calculateHash()
				hash2 := arch.hash

				Expect(hash1).To(Equal(hash2))
			})
		})

		Context("when hash calculation returns an error", func() {
			BeforeEach(func() {
				mockHashCalculator = func(_ string) (hash string, err error) {
					return "", fmt.Errorf("fake error")
				}
				arch.hashCalculator = mockHashCalculator
			})

			It("should panic with an error", func() {
				Expect(arch.calculateHash).To(Panic())
			})
		})
	})

	Describe("detectAvailableArchitectures", func() {
		var tempDir string
		var err error

		BeforeEach(func() {
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tempDir)).To(Succeed())
		})

		It("shouldn't find available architectures", func() {
			// Create some sample files
			Expect(os.WriteFile(filepath.Join(tempDir, "test1"), []byte("amd64"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tempDir, "test2"), []byte("arm64"), 0o600)).To(Succeed())

			err = detectAvailableArchitectures(filepath.Join(tempDir, "manager_*"))
			Expect(err).ToNot(HaveOccurred())

			architectures := GetAvailableArchitectures()
			Expect(architectures).To(BeEmpty())
		})

		It("should find available architectures", func() {
			expectedArchitectures := []string{"amd64", "arm64"}

			// Create some sample files
			Expect(os.WriteFile(filepath.Join(tempDir, "manager_amd64"), []byte("amd64"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tempDir, "manager_arm64"), []byte("arm64"), 0o600)).To(Succeed())

			err = detectAvailableArchitectures(filepath.Join(tempDir, "manager_*"))
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				architectures := GetAvailableArchitectures()
				Expect(architectures).NotTo(BeEmpty())
				for _, a := range architectures {
					g.Expect(slices.Contains(expectedArchitectures, a.GoArch)).To(BeTrue())
					g.Expect(a.GetHash()).ToNot(BeEmpty())
				}
			}).Should(Succeed())
		})
	})
})
