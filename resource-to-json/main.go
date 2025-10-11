package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	yassv1 "github.com/ESA-PhiLab/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	namespace := goutils.EnvRequired[string]("NAMESPACE")
	dstFilename := goutils.Env("DST_FILE", "/tmp/exported-resource.json")
	resourceName := goutils.EnvRequired[string]("RESOURCE_NAME")
	resourceKind := goutils.EnvRequired[string]("RESOURCE_KIND")
	slog.Info("Trying to extract kubernetes resource", "namespace", namespace, "name", resourceName, "kind", resourceKind, "toFilename", dstFilename)
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	goutils.ExitOnError(err, 2)
	err = yassv1.AddToScheme(scheme)
	goutils.ExitOnError(err, 2)
	cfg := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(fmt.Errorf("creating k8s client: %w", err))
	}
	var jsonObj *map[string]any
	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
	switch strings.ToLower(resourceKind) {
	case "fsnode":
		jsonObj, err = handleFsNodeResource(ctx, k8sClient, namespacedName)
	case "experiment":
		jsonObj, err = handleExperimentResource(ctx, k8sClient, namespacedName)
	default:
		panic(fmt.Sprintf("dont know how to handle kind '%s'", resourceKind))
	}
	if err != nil {
		panic(fmt.Sprintf("error handling kind %s for %s :: %s", resourceKind, namespacedName, err))
	}
	slog.Info("Got resource", "resource", namespacedName)
	buff, err := json.Marshal(jsonObj)
	if err != nil {
		panic(fmt.Sprintf("cannot convert %T to json :: %s", jsonObj, err))
	}
	slog.Info("saving data to json file", "filename", dstFilename)
	err = os.WriteFile(dstFilename, buff, 0o744)
	if err != nil {
		panic(fmt.Sprintf("cannot save file %s :: %s", dstFilename, err))
	}
}

func handleFsNodeResource(ctx context.Context, k8sClient client.Client, namespacedName types.NamespacedName) (*map[string]any, error) {
	jsonObj := make(map[string]any)
	obj := &yassv1.FsNode{}
	err := k8sClient.Get(ctx, namespacedName, obj)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource %T %s", obj, namespacedName))
	}
	jsonObj["name"] = obj.Name
	jsonObj["hardwareSpec"] = obj.Spec.HardwareSpec
	jsonObj["orbit"] = obj.Spec.Orbit
	jsonObj["rotation"] = obj.Spec.Rotation
	jsonObj["engine"] = obj.Spec.Engine
	jsonObj["agent"] = obj.Spec.Agent
	return &jsonObj, nil
}

func handleExperimentResource(ctx context.Context, k8sClient client.Client, namespacedName types.NamespacedName) (*map[string]any, error) {
	jsonObj := make(map[string]any)
	experiment := &yassv1.Experiment{}
	err := k8sClient.Get(ctx, namespacedName, experiment)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource %T %s", experiment, namespacedName))
	}
	jsonObj["name"] = experiment.Name
	jsonObj["simulationStartTime"] = experiment.Spec.SimulationStartTime

	layout := &yassv1.Layout{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: experiment.Spec.LayoutDefRef}, layout)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource Layout %s", experiment.Spec.LayoutDefRef))
	}
	jsonObj["layout"] = layout.Spec

	expDef := &yassv1.ExperimentDefinition{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: experiment.Spec.ExperimentDefRef}, expDef)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource ExperimentDefinition %s", experiment.Spec.ExperimentDefRef))
	}
	jsonObj["experimentDefinition"] = expDef.Spec

	return &jsonObj, nil
}
