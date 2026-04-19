package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/go-common/cmodel"
	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
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
	goutils.ExitOnErrorf(startup.InitSlog(), 1, "cannot initialize slog")
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	namespace := goutils.EnvRequired[string]("NAMESPACE")
	dstDir := goutils.Env("DST_DIR", "/mnt/shared")
	resourceName := goutils.EnvRequired[string]("RESOURCE_NAME")
	resourceKind := goutils.EnvRequired[string]("RESOURCE_KIND")
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
	var jsonObj any
	namespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
	exportedResources := map[string]any{}
	switch strings.ToLower(resourceKind) {
	case "fsnode":
		var hwSpec *yassv1.HardwareSpec
		jsonObj, hwSpec, err = handleFsNodeResource(ctx, k8sClient, namespacedName)
		exportedResources["fs-node.json"] = jsonObj
		exportedResources["hardware.json"] = hwSpec
	case "experiment":
		jsonObj, err = handleExperimentResource(ctx, k8sClient, namespacedName)
		exportedResources["experiment.json"] = jsonObj
	case "hardwareDefinition":
		jsonObj, err = handleHardwareDefinitionResource(ctx, k8sClient, namespacedName)
		exportedResources["hardware.json"] = jsonObj

	default:
		panic(fmt.Sprintf("dont know how to handle kind '%s'", resourceKind))
	}
	if err != nil {
		panic(fmt.Sprintf("error handling kind %s for %s :: %s", resourceKind, namespacedName, err))
	}
	for fn, obj := range exportedResources {
		dstFilename := path.Join(dstDir, fn)
		slog.Info("Resource exporting", "resourceType", fmt.Sprintf("%T", obj), "filename", dstFilename)
		buff, err := json.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("cannot convert %T to json :: %s", obj, err))
		}
		err = os.WriteFile(dstFilename, buff, 0o744)
		fmt.Printf("JSON %s:\n\n%s\n\n", dstFilename, string(buff))
		if err != nil {
			panic(fmt.Sprintf("cannot save file %s :: %s", dstFilename, err))
		}
		slog.Info("Resource exported successfully", "resourceType", fmt.Sprintf("%T", obj), "filename", dstFilename)
	}
	slog.Info("Completed")
}

func handleFsNodeResource(
	ctx context.Context, k8sClient client.Client, namespacedName types.NamespacedName,
) (*cmodel.FsNode, *yassv1.HardwareSpec, error) {
	ret := &cmodel.FsNode{}
	obj := &yassv1.FsNode{}
	err := k8sClient.Get(ctx, namespacedName, obj)
	if err != nil {
		return nil, nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource %T %s", obj, namespacedName))
	}

	ret.Name = obj.Name
	if obj.Spec.Orbit != nil && obj.Spec.Orbit.TLE != nil {
		ret.TLE = obj.Spec.Orbit.TLE
	}
	if obj.Spec.EarthPosition != nil {
		ret.Geo = &cmodel.GeoCoordinates{
			Lat: obj.Spec.EarthPosition.Lat,
			Lng: obj.Spec.EarthPosition.Lng,
		}
	}
	if obj.Spec.Rotation != nil {
		ret.Rotation.Yaw = obj.Spec.Rotation.Yaw
		ret.Rotation.Roll = obj.Spec.Rotation.Roll
		ret.Rotation.Pitch = obj.Spec.Rotation.Pitch
	}
	return ret, obj.Spec.HardwareSpec, nil
}
func handleHardwareDefinitionResource(
	ctx context.Context, k8sClient client.Client, namespacedName types.NamespacedName,
) (*yassv1.HardwareDefinition, error) {
	obj := &yassv1.HardwareDefinition{}
	err := k8sClient.Get(ctx, namespacedName, obj)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource %T %s", obj, namespacedName))
	}
	ret := obj.DeepCopy()
	return ret, nil
}

func handleExperimentResource(
	ctx context.Context, k8sClient client.Client, namespacedName types.NamespacedName,
) (*cmodel.ExperimentDefinition, error) {
	js := &cmodel.ExperimentDefinition{}
	experiment := &yassv1.Experiment{}
	err := k8sClient.Get(ctx, namespacedName, experiment)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource %T %s", experiment, namespacedName))
	}
	js.Name = experiment.Name
	if !experiment.Spec.SimulationStartTime.IsZero() {
		js.StartTime = &experiment.Spec.SimulationStartTime.Time
	}

	layout := &yassv1.Layout{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: experiment.Spec.LayoutDefRef}, layout)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error getting kubernetes resource Layout %s", experiment.Spec.LayoutDefRef))
	}
	for _, nodeSpec := range layout.Spec {
		node := toNode(nodeSpec)
		js.FsNodes = append(js.FsNodes, node)
	}

	expDef := &yassv1.ExperimentDefinition{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: experiment.Spec.ExperimentDefRef}, expDef)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("error getting kubernetes resource ExperimentDefinition %s", experiment.Spec.ExperimentDefRef))
	}
	maxDur := strings.TrimSpace(expDef.Spec.MaxDuration)
	js.MaxDuration = nil
	if maxDur != "" {
		dur, err := time.ParseDuration(maxDur)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot convert '%s' to duration", maxDur)
		}
		js.MaxDuration = &dur
	}

	return js, nil
}

func toNode(spec yassv1.LayoutSatSpec) cmodel.ExperimentFsNode {
	ret := cmodel.ExperimentFsNode{
		Name:     spec.FsNodeName,
		Rotation: cmodel.Rotation{},
	}
	if spec.Orbit != nil && spec.Orbit.TLE != nil {
		ret.TLE = spec.Orbit.TLE
	}
	if spec.EarthPosition != nil {
		ret.Geo = &cmodel.GeoCoordinates{
			Lat: spec.EarthPosition.Lat,
			Lng: spec.EarthPosition.Lng,
		}
	}
	if spec.Rotation != nil {
		ret.Rotation.Yaw = spec.Rotation.Yaw
		ret.Rotation.Roll = spec.Rotation.Roll
		ret.Rotation.Pitch = spec.Rotation.Pitch
	}
	return ret
}
