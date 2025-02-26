// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package unstructuredscheme

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})
}

func NewNegotiatedSerializer() runtime.NegotiatedSerializer {
	return &negotiatedSerializer{}
}

type negotiatedSerializer struct{}

func (s negotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	return []runtime.SerializerInfo{
		{
			MediaType:        "application/json",
			MediaTypeType:    "application",
			MediaTypeSubType: "json",
			EncodesAsText:    true,
			Serializer:       json.NewSerializer(json.DefaultMetaFactory, creator{scheme}, typer{scheme}, false),
			StreamSerializer: &runtime.StreamSerializerInfo{
				EncodesAsText: true,
				Serializer:    json.NewSerializer(json.DefaultMetaFactory, scheme, scheme, false),
				Framer:        json.Framer,
			},
		},
	}
}

func (s negotiatedSerializer) EncoderForVersion(encoder runtime.Encoder, gv runtime.GroupVersioner) runtime.Encoder {
	return runtime.WithVersionEncoder{
		Version:     gv,
		Encoder:     encoder,
		ObjectTyper: typer{scheme},
	}
}

func (s negotiatedSerializer) DecoderToVersion(decoder runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return decoder
}

type creator struct {
	objCreator runtime.ObjectCreater
}

func (c creator) New(kind schema.GroupVersionKind) (runtime.Object, error) {
	obj, err := c.objCreator.New(kind)
	if err == nil {
		return obj, nil
	}

	obj = &unstructured.Unstructured{}
	obj.GetObjectKind().SetGroupVersionKind(kind)
	return obj, nil
}

type typer struct {
	typer runtime.ObjectTyper
}

func (t typer) ObjectKinds(obj runtime.Object) ([]schema.GroupVersionKind, bool, error) {
	kinds, unversioned, err := t.typer.ObjectKinds(obj)
	if err == nil {
		return kinds, unversioned, nil
	}

	if _, ok := obj.(runtime.Unstructured); ok && !obj.GetObjectKind().GroupVersionKind().Empty() {
		return []schema.GroupVersionKind{obj.GetObjectKind().GroupVersionKind()}, false, nil
	}
	return nil, false, err
}

func (t typer) Recognizes(_ schema.GroupVersionKind) bool {
	return true
}
