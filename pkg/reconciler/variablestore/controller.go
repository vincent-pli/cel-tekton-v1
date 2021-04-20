/*
Copyright 2019 The Knative Authors

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

package variablestore

import (
	"context"
	"os"
	"io/ioutil"
	"strconv"

	"knative.dev/pkg/tracker"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	runinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/run"
	runreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1alpha1/run"
	pipelinecontroller "github.com/tektoncd/pipeline/pkg/controller"
	variablestorev1alpha1 "github.com/vincentpli/cel-tekton/pkg/apis/variablestores/v1alpha1"
	variablestoreclient "github.com/vincentpli/cel-tekton/pkg/client/injection/client"
	"k8s.io/client-go/tools/cache"
)

const (
	CAcertPath  = "/etc/redis-client/ca.crt"
	USERNAME = "USERNAME"
	PASSWORD = "PASSOWRD"
	DB = "DB"
	ADDR = "ADDR"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)

	variablestoreclientset := variablestoreclient.Get(ctx)

	runInformer := runinformer.Get(ctx)

	//Setup client of Redis
	caCert, err := ioutil.ReadFile(CAcertPath)
	if err != nil {
		logger.Fatalf("Failed to read CA cert for Redis client: %v", err)
		os.Exit(1)
	}

	if username, ok := os.LookupEnv(USERNAME); !ok {
		logger.Fatal("Failed to read USERNAME of Redis from env variables: %v")
		os.Exit(1)
	}

	if password, ok := os.LookupEnv(PASSWORD); !ok {
		logger.Fatal("Failed to read PASSWORD of Redis from env variables: %v")
		os.Exit(1)
	}

	if addr, ok := os.LookupEnv(ADDR); !ok {
		logger.Fatal("Failed to read ADDR of Redis from env variables: %v")
		os.Exit(1)
	}

	if db, ok := os.LookupEnv(db); !ok {
		db = "0"
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username
		Password: password,
		DB:       strconv.Atoi(db),
		TLSConfig: &tls.Config{
			RootCAs: caCertPool,
		},
	})

	r := &Reconciler{
		variablestoreClientSet: variablestoreclientset,
		runLister:              runInformer.Lister(),
		rdb: rdb,
	}

	impl := runreconciler.NewImpl(ctx, r)
	r.Tracker = tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

	logger.Info("Setting up event handlers.")

	runInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: pipelinecontroller.FilterRunRef(variablestorev1alpha1.SchemeGroupVersion.String(), "VariableStore"),
		Handler:    controller.HandleAll(impl.Enqueue),
	})

	return impl
}
