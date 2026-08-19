// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	frrk8sv1beta1 "github.com/metallb/frr-k8s/api/v1beta1"
	"github.com/metallb/frrk8stests/pkg/routes"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.universe.tf/e2etest/pkg/frr"
	frrcontainer "go.universe.tf/e2etest/pkg/frr/container"
	"go.universe.tf/e2etest/pkg/ipfamily"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

func ValidateFRRNotPeeredWithNodes(nodes []corev1.Node, c *frrcontainer.FRR, ipFamily ipfamily.Family) {
	for _, node := range nodes {
		ginkgo.By(fmt.Sprintf("checking node %s is not peered with the frr instance %s", node.Name, c.Name))
		Eventually(func() error {
			neighbors, err := frr.NeighborsInfo(c)
			Expect(err).NotTo(HaveOccurred())
			err = frr.NeighborsMatchNodes([]corev1.Node{node}, neighbors, ipFamily, c.RouterConfig.VRF)
			return err
		}, 4*time.Minute, 1*time.Second).Should(MatchError(ContainSubstring("not established")))
	}
}

func ValidateFRRPeeredWithNodes(nodes []corev1.Node, c *frrcontainer.FRR, ipFamily ipfamily.Family) {
	ginkgo.By(fmt.Sprintf("checking nodes are peered with the frr instance %s", c.Name))
	Eventually(func() error {
		neighbors, err := frr.NeighborsInfo(c)
		Expect(err).NotTo(HaveOccurred())
		err = frr.NeighborsMatchNodes(nodes, neighbors, ipFamily, c.RouterConfig.VRF)
		if err != nil {
			return fmt.Errorf("failed to match neighbors for %s, %w", c.Name, err)
		}
		return nil
	}, 4*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
}

func ValidatePrefixesForNeighbor(neigh frrcontainer.FRR, nodes []v1.Node, prefixes ...string) {
	ValidatePrefixesForNeighborVRF(neigh, nodes, neigh.RouterConfig.VRF, prefixes...)
}

// Validates the given neighbor has the prefixes towards the given VRF
func ValidatePrefixesForNeighborVRF(neigh frrcontainer.FRR, nodes []v1.Node, vrfName string, prefixes ...string) {
	ginkgo.By(fmt.Sprintf("checking prefixes %v for %s", prefixes, neigh.Name))
	Eventually(func() error {
		for _, prefix := range prefixes {
			err := routes.CheckNeighborHasPrefix(neigh, vrfName, prefix, nodes)
			if err != nil {
				return fmt.Errorf("Neigh %s does not have prefix %s: %w", neigh.Name, prefix, err)
			}
		}
		return nil
	}, time.Minute, time.Second).ShouldNot(HaveOccurred())
}

func ValidatePrefixesForNeighborWithNextHop(neigh frrcontainer.FRR, nextHop string, prefixes ...string) {
	ginkgo.By(fmt.Sprintf("checking prefixes %v for %s with next hop %s", prefixes, neigh.Name, nextHop))
	Eventually(func() error {
		for _, prefix := range prefixes {
			err := routes.CheckNeighborHasPrefixWithNextHop(neigh, prefix, nextHop)
			if err != nil {
				return fmt.Errorf("neigh %s does not have prefix %s with next hop %s: %w", neigh.Name, prefix, nextHop, err)
			}
		}
		return nil
	}, time.Minute, time.Second).ShouldNot(HaveOccurred())
}

func ValidateNeighborNoPrefixes(neigh frrcontainer.FRR, nodes []v1.Node, prefixes ...string) {
	ValidateNeighborNoPrefixesVRF(neigh, nodes, neigh.RouterConfig.VRF, prefixes...)
}

func ValidateNeighborNoPrefixesVRF(neigh frrcontainer.FRR, nodes []v1.Node, vrfName string, prefixes ...string) {
	ginkgo.By(fmt.Sprintf("checking prefixes %v not announced to %s", prefixes, neigh.Name))
	Eventually(func() error {
		for _, prefix := range prefixes {
			err := routes.CheckNeighborHasPrefix(neigh, vrfName, prefix, nodes)
			if err != nil {
				return fmt.Errorf("Neigh %s does not have prefix %s: %w", neigh.Name, prefix, err)
			}
		}
		return nil
	}, 5*time.Second, time.Second).Should(
		MatchError(
			Or(ContainSubstring("route not found"),
				ContainSubstring("not found in nodes"))))

}

func ValidateNeighborCommunityPrefixes(neigh frrcontainer.FRR, community string, prefixes []string, ipfam ipfamily.Family) {
	Eventually(func() error {
		routes, err := frr.RoutesForCommunity(neigh, community, ipfam)
		if err != nil {
			return err
		}

		communityPrefixes := map[string]struct{}{}
		for p := range routes {
			communityPrefixes[p] = struct{}{}
		}

		for _, prefix := range prefixes {
			_, ok := communityPrefixes[prefix]
			if !ok {
				return fmt.Errorf("prefix %s not found in neighbor %s community %s unmatched routes %s", prefix, neigh.Name, community, communityPrefixes)
			}
			delete(communityPrefixes, prefix)
		}

		if len(communityPrefixes) != 0 {
			return fmt.Errorf("routes %s for community %s were not matched for neighbor %s", communityPrefixes, community, neigh.Name)
		}

		return nil
	}, 5*time.Second, time.Second).ShouldNot(HaveOccurred())
}

func ValidateNodesHaveRoutesVRF(pods []*v1.Pod, neigh frrcontainer.FRR, vrf string, prefixes ...string) {
	ginkgo.By(fmt.Sprintf("Checking routes %v from %s", prefixes, neigh.Name))
	Eventually(func() error {
		for _, prefix := range prefixes {
			for _, pod := range pods {
				if !routes.PodHasPrefixFromContainer(pod, neigh, vrf, prefix) {
					return fmt.Errorf("pod %s does not have prefix %s from %s", pod.Name, prefix, neigh.Name)
				}
			}
		}
		return nil
	}, time.Minute, time.Second).ShouldNot(HaveOccurred())
}

func ValidateNodesHaveRoutes(pods []*v1.Pod, neigh frrcontainer.FRR, prefixes ...string) {
	ValidateNodesHaveRoutesVRF(pods, neigh, neigh.RouterConfig.VRF, prefixes...)
}

func ValidateNodesDoNotHaveRoutes(pods []*v1.Pod, neigh frrcontainer.FRR, prefixes ...string) {
	ginkgo.By(fmt.Sprintf("Checking routes %v not injected from %s", prefixes, neigh.Name))
	shouldPassConsistently(func() error {
		for _, prefix := range prefixes {
			for _, pod := range pods {
				if routes.PodHasPrefixFromContainer(pod, neigh, neigh.RouterConfig.VRF, prefix) {
					return fmt.Errorf("pod %s has prefix %s from %s", pod.Name, prefix, neigh.Name)
				}
			}
		}
		return nil
	})
}

func ValidateNeighborLocalPrefForPrefix(neigh frrcontainer.FRR, prefix string, expectedLocalPref uint32, ipfam ipfamily.Family) {
	if !strings.Contains(neigh.Name, "ibgp") {
		return // localPref is valid only for iBGP connections
	}

	ginkgo.By(fmt.Sprintf("Checking localPref for prefix %s on neighbor %s", prefix, neigh.Name))
	Eventually(func() error {
		localPrefix, err := frr.LocalPrefForPrefix(neigh, prefix, ipfam)
		if err != nil {
			return err
		}

		if localPrefix != expectedLocalPref {
			return fmt.Errorf("local pref %d for prefix %s on neighbor %s does not equal %d", localPrefix, prefix, neigh.Name, expectedLocalPref)
		}

		return nil
	}, 5*time.Second, time.Second).ShouldNot(HaveOccurred())
}

func ValidateNeighborAsPathPrependForPrefix(neigh frrcontainer.FRR, prefix string, expectedAsPathPrepend uint8, ipfam ipfamily.Family) {
	if !strings.Contains(neigh.Name, "ebgp") {
		return // asPathPrepend is valid only for eBGP connections
	}

	ginkgo.By(fmt.Sprintf("Checking asPathPrepend for prefix %s on neighbor %s", prefix, neigh.Name))
	Eventually(func() error {
		asPathPrepend, err := AsPathPrependForPrefix(neigh, prefix, ipfam)
		if err != nil {
			return err
		}

		if asPathPrepend != expectedAsPathPrepend {
			return fmt.Errorf("asPathPrepend %d for prefix %s on neighbor %s does not equal %d", asPathPrepend, prefix, neigh.Name, expectedAsPathPrepend)
		}

		return nil
	}, 5*time.Second, time.Second).ShouldNot(HaveOccurred())
}

// TODO(metallb/metallb#3120): Extend MetalLB's FRR.Route struct to natively parse and store AS-Path prepend data.
func AsPathPrependForPrefix(neigh frrcontainer.FRR, prefix string, ipfam ipfamily.Family) (uint8, error) {
	// 1. Run the command inside the external FRR container
	cmd := fmt.Sprintf("vtysh -c 'show bgp %s unicast %s json'", ipfam, prefix)
	out, err := neigh.Executor.Exec("sh", "-c", cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to execute vtysh: %w", err)
	}

	// 2. Define a minimal struct to parse just the AS Path from FRR's JSON output
	var routeData struct {
		Paths []struct {
			AsPath struct {
				String string `json:"string"`
			} `json:"aspath"`
		} `json:"paths"`
	}

	// 3. Unmarshal the JSON
	if err := json.Unmarshal([]byte(out), &routeData); err != nil {
		return 0, fmt.Errorf("failed to parse FRR JSON: %w", err)
	}

	if len(routeData.Paths) == 0 {
		return 0, fmt.Errorf("no paths found for prefix %s", prefix)
	}

	// 4. Extract the AS Path string (e.g., "65000 65000 65000 65000")
	asPathStr := routeData.Paths[0].AsPath.String

	if strings.TrimSpace(asPathStr) == "" {
		return 0, nil // No ASNs in the path
	}

	// 5. Count the ASNs.
	// If the base ASN is added once natively, and we prepend 3 times, there are 4 ASNs total.
	// So prependCount = Total ASNs - 1
	asnList := strings.Fields(asPathStr)
	prependCount := len(asnList) - 1

	return uint8(prependCount), nil
}

func checkBFDConfigPropagated(nodeConfig frrk8sv1beta1.BFDProfile, peerConfig frr.BFDPeer) error {
	if peerConfig.Status != "up" {
		return fmt.Errorf("peer status not up")
	}
	if peerConfig.RemoteReceiveInterval != int(*nodeConfig.ReceiveInterval) {
		return fmt.Errorf("remoteReceiveInterval: expecting %d, got %d", nodeConfig.ReceiveInterval, peerConfig.RemoteReceiveInterval)
	}
	if peerConfig.RemoteTransmitInterval != int(*nodeConfig.TransmitInterval) {
		return fmt.Errorf("remoteTransmitInterval: expecting %d, got %d", nodeConfig.TransmitInterval, peerConfig.RemoteTransmitInterval)
	}
	if peerConfig.RemoteEchoReceiveInterval != int(*nodeConfig.EchoInterval) {
		return fmt.Errorf("echoInterval: expecting %d, got %d", nodeConfig.EchoInterval, peerConfig.RemoteEchoReceiveInterval)
	}
	return nil
}

// shouldPassConsistently checks for the failure to happen
// and then checks it consistently.
func shouldPassConsistently(toCheck func() error) {
	Eventually(func() error {
		return toCheck()
	}, 2*time.Minute, time.Second).ShouldNot(HaveOccurred())
	Consistently(func() error {
		return toCheck()
	}, 5*time.Second, time.Second).ShouldNot(HaveOccurred())
}
