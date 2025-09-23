package k8s

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func NewClient(kubeconfig string) (*kubernetes.Clientset, error) {
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

func ListPodImages(clientset *kubernetes.Clientset, namespace string) ([]string, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var images []string
	for _, pod := range pods.Items {
		// Main containers
		for _, container := range pod.Spec.Containers {
			images = append(images, container.Image)
		}

		// Init containers
		for _, container := range pod.Spec.InitContainers {
			images = append(images, container.Image)
		}
	}

	return images, nil
}

// ListPodImagesFiltered lists images from pods with namespace include/exclude semantics.
// If allNamespaces is true, it lists from all namespaces and applies include/exclude on pod.Namespace.
// If allNamespaces is false, it lists only from baseNamespace and ignores include/exclude lists.
func ListPodImagesFiltered(clientset *kubernetes.Clientset, allNamespaces bool, baseNamespace string, includeNamespaces, excludeNamespaces []string) ([]string, error) {
	listNamespace := baseNamespace
	if allNamespaces {
		listNamespace = metav1.NamespaceAll
	}

	pods, err := clientset.CoreV1().Pods(listNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Compile namespace matchers: regex if derlenebilir, aksi halde prefix
	incMatchers, _ := compileNamespaceMatchers(includeNamespaces)
	excMatchers, _ := compileNamespaceMatchers(excludeNamespaces)

	var images []string
	for _, pod := range pods.Items {
		ns := pod.Namespace
		if allNamespaces {
			if len(incMatchers) > 0 && !namespaceMatchesAny(ns, incMatchers) {
				continue
			}
			if len(excMatchers) > 0 && namespaceMatchesAny(ns, excMatchers) {
				continue
			}
		}

		for _, container := range pod.Spec.Containers {
			images = append(images, container.Image)
		}
		for _, container := range pod.Spec.InitContainers {
			images = append(images, container.Image)
		}
	}
	return images, nil
}

// ImageInfo contains an image and its source owner information
type ImageInfo struct {
	Image      string `json:"image" yaml:"image"`
	Namespace  string `json:"namespace" yaml:"namespace"`
	SourceKind string `json:"sourceKind" yaml:"sourceKind"`
	SourceName string `json:"sourceName" yaml:"sourceName"`
}

// ListPodImagesWithSource lists images and includes their owning controller (e.g., Job, ReplicaSet) or Pod if standalone.
func ListPodImagesWithSource(clientset *kubernetes.Clientset, allNamespaces bool, baseNamespace string, includeNamespaces, excludeNamespaces []string) ([]ImageInfo, error) {
	listNamespace := baseNamespace
	if allNamespaces {
		listNamespace = metav1.NamespaceAll
	}

	pods, err := clientset.CoreV1().Pods(listNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	incMatchers, _ := compileNamespaceMatchers(includeNamespaces)
	excMatchers, _ := compileNamespaceMatchers(excludeNamespaces)

	var results []ImageInfo
	for _, pod := range pods.Items {
		ns := pod.Namespace
		if allNamespaces {
			if len(incMatchers) > 0 && !namespaceMatchesAny(ns, incMatchers) {
				continue
			}
			if len(excMatchers) > 0 && namespaceMatchesAny(ns, excMatchers) {
				continue
			}
		}

		kind := "Pod"
		owner := pod.Name
		if len(pod.OwnerReferences) > 0 {
			kind = pod.OwnerReferences[0].Kind
			owner = pod.OwnerReferences[0].Name
			// Try to resolve top owner (e.g., ReplicaSet -> Deployment, Job -> CronJob)
			if topKind, topName, err := ResolveTopOwner(clientset, ns, kind, owner); err == nil {
				if topKind != "" {
					kind = topKind
					owner = topName
				}
			}
		}

		for _, c := range pod.Spec.Containers {
			results = append(results, ImageInfo{Image: c.Image, Namespace: ns, SourceKind: kind, SourceName: owner})
		}
		for _, c := range pod.Spec.InitContainers {
			results = append(results, ImageInfo{Image: c.Image, Namespace: ns, SourceKind: kind, SourceName: owner})
		}
	}
	return results, nil
}

// ResolveTopOwner follows common controller chains to the top-level owner when available.
// Supported chains: ReplicaSet -> Deployment, Job -> CronJob. Falls back to the provided kind/name.
func ResolveTopOwner(clientset *kubernetes.Clientset, namespace, kind, name string) (string, string, error) {
	switch kind {
	case "ReplicaSet":
		rs, err := clientset.AppsV1().ReplicaSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return kind, name, nil
		}
		if len(rs.OwnerReferences) > 0 {
			or := rs.OwnerReferences[0]
			if or.Kind == "Deployment" {
				return or.Kind, or.Name, nil
			}
		}
		return kind, name, nil
	case "Job":
		job, err := clientset.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return kind, name, nil
		}
		if len(job.OwnerReferences) > 0 {
			or := job.OwnerReferences[0]
			if or.Kind == "CronJob" {
				return or.Kind, or.Name, nil
			}
		}
		return kind, name, nil
	default:
		return kind, name, nil
	}
}

type namespaceMatcher struct {
	isRegex bool
	prefix  string
	re      *regexp.Regexp
}

func (m namespaceMatcher) match(s string) bool {
	if m.isRegex {
		return m.re.MatchString(s)
	}
	return strings.HasPrefix(s, m.prefix)
}

func compileNamespaceMatchers(patterns []string) ([]namespaceMatcher, error) {
	var matchers []namespaceMatcher
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			matchers = append(matchers, namespaceMatcher{isRegex: false, prefix: p})
			continue
		}
		matchers = append(matchers, namespaceMatcher{isRegex: true, re: re})
	}
	return matchers, nil
}

func namespaceMatchesAny(s string, matchers []namespaceMatcher) bool {
	for _, m := range matchers {
		if m.match(s) {
			return true
		}
	}
	return false
}
