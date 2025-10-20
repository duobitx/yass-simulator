package k8s

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ResourceToJson(ctx context.Context, k8sClient client.Client, namespace, name string, obj client.Object) ([]byte, error) {
	objKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	err := k8sClient.Get(ctx, objKey, obj)
	if err != nil {
		return nil, err
	}
	buff, err := json.Marshal(obj)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("cannot marshal object of type %T to json", obj))
	}
	return buff, nil
}
