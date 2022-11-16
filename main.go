//Copyright © 2019-2022 Dell Inc. or its subsidiaries. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//      http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
//go:generate go generate ./core

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dell/csi-vxflexos/v2/k8sutils"
	"github.com/dell/csi-vxflexos/v2/provider"
	"github.com/dell/csi-vxflexos/v2/service"
	"github.com/dell/gocsi"
	"github.com/sirupsen/logrus"
)

// main is ignored when this package is built as a go plug-in
func main() {
	logger := logrus.New()
	service.Log = logger
	// Always set X_CSI_DEBUG to false irrespective of what user has specified
	_ = os.Setenv(gocsi.EnvVarDebug, "false")
	// We always want to enable Request and Response logging(no reason for users to control this)
	_ = os.Setenv(gocsi.EnvVarReqLogging, "true")
	_ = os.Setenv(gocsi.EnvVarRepLogging, "true")

	// zhou: "--array-config=/vxflexos-config/config" specified in csi both controller and node plugin.
	//
	//       Secret volume "vxflexos-config" will be mounted in this path.
	//       The Secret is created from "powerflex-csi/sample/config.yaml"

	arrayConfigfile := flag.String("array-config", "", "yaml file with array(s) configuration")

	// zhou: "--driver-config-params=/vxflexos-config-params/driver-config-params.yaml"
	//       specified in csi both controller and node plugin.
	//
	//       ConfigMap volume "vxflexos-config-params" will be mounted in this path.
	//       The ConfigMap just contains
	//       CSI_LOG_LEVEL: "debug"
	//       CSI_LOG_FORMAT: "TEXT"

	driverConfigParamsfile := flag.String("driver-config-params", "", "yaml file with driver config params")

	// zhou: "--leader-election" specified in csi controller plugin only.

	enableLeaderElection := flag.Bool("leader-election", false, "boolean to enable leader election")
	leaderElectionNamespace := flag.String("leader-election-namespace", "", "namespace where leader election lease will be created")

	// zhou: optional

	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()

	if *arrayConfigfile == "" {
		fmt.Fprintf(os.Stderr, "array-config argument is mandatory")
		os.Exit(1)
	}

	if *driverConfigParamsfile == "" {
		fmt.Fprintf(os.Stderr, "driver-config-params argument is mandatory")
		os.Exit(1)
	}

	// zhou: not struct, just module's global variables.

	service.ArrayConfigFile = *arrayConfigfile
	service.DriverConfigParamsFile = *driverConfigParamsfile
	service.KubeConfig = *kubeconfig

	// Run the service as a pre-init step.
	if os.Getenv(gocsi.EnvVarMode) == "mdm-info" {
		fmt.Fprintf(os.Stdout, "PowerFlex Container Storage Interface (CSI) Plugin starting in pre-init mode.")
		svc := service.NewPreInitService()
		err := svc.PreInit()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to complete pre-init: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// zhou: service name is "csi-vxflexos.dellemc.com"
	//       define fucntion "run" here.
	//
	//       "gocsi.Run()", will create a gRPC server, and register handlers defined in
	//       "provider.New()" to it.
	//       Then, all the gPRC will be handled by "service.New()"

	run := func(ctx context.Context) {
		gocsi.Run(ctx, service.Name, "A PowerFlex Container Storage Interface (CSI) Plugin",
			usage, provider.New())
	}
	if !*enableLeaderElection {
		run(context.Background())
	} else {
		// zhou: "driver-csi-vxflexos-dellemc-com"
		driverName := strings.Replace(service.Name, ".", "-", -1)
		lockName := fmt.Sprintf("driver-%s", driverName)
		err := k8sutils.CreateKubeClientSet()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to create clientset for leader election: %v", err)
			os.Exit(1)
		}
		service.K8sClientset = k8sutils.Clientset
		// Attempt to become leader and start the driver
		k8sutils.LeaderElection(&k8sutils.Clientset, lockName, *leaderElectionNamespace, run)
	}
}

const usage = `    X_CSI_VXFLEXOS_SDCGUID
        Specifies the GUID of the SDC. This is only used by the Node Service,
        and removes a need for calling an external binary to retrieve the GUID.
        If not set, the external binary will be invoked.

        The default value is empty.

    X_CSI_VXFLEXOS_THICKPROVISIONING
        Specifies whether thick provisiong should be used when creating volumes.

        The default value is false.

    X_CSI_VXFLEXOS_ENABLESNAPSHOTCGDELETE
        When a snapshot is deleted, if it is a member of a Consistency Group, enable automatic deletion
        of all snapshots in the consistency group.

        The default value is false.

    X_CSI_VXFLEXOS_ENABLELISTVOLUMESNAPSHOTS
        When listing volumes, if this option is is enabled, then volumes and snapshots will be returned.
        Otherwise only volumes are returned.

        The default value is false.
`
