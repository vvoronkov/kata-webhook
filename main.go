package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sirupsen/logrus"
	kwhhttp "github.com/slok/kubewebhook/v2/pkg/http"
	kwhlogrus "github.com/slok/kubewebhook/v2/pkg/log/logrus"
	kwhmodel "github.com/slok/kubewebhook/v2/pkg/model"
	kwhmutating "github.com/slok/kubewebhook/v2/pkg/webhook/mutating"
)

func annotatePodMutator(_ context.Context, ar *kwhmodel.AdmissionReview, obj metav1.Object) (*kwhmutating.MutatorResult, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		// If not a pod just continue the mutation chain(if there is one) and don't do nothing.
		return &kwhmutating.MutatorResult{}, nil
	}

	for k, v := range whPolicy.nsBlacklist {
		fmt.Printf("key[%s] value[%s]\n", k, v)
		match, _ := regexp.MatchString(k, ar.Namespace)

		if match {
			fmt.Println("blacklisted namespace: ", ar.Namespace)
		    return &kwhmutating.MutatorResult{}, nil
		}
	}

	if whPolicy.nsBlacklist[ar.Namespace] {
		fmt.Println("blacklisted namespace: ", ar.Namespace)
		return &kwhmutating.MutatorResult{}, nil
	}

	if validRegex != nil {
		if !validRegex.MatchString(ar.Namespace) {
			fmt.Println("namespace doesn't match regex expression", ar.Namespace)
		    return &kwhmutating.MutatorResult{}, nil
		}
	}

	// We cannot support --net=host in Kata
	// https://github.com/kata-containers/documentation/blob/master/Limitations.md#docker---nethost
	if pod.Spec.HostNetwork {
		fmt.Println("host network: ", pod.GetNamespace(), pod.GetName())
        return &kwhmutating.MutatorResult{}, nil
	}

	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].SecurityContext != nil && pod.Spec.Containers[i].SecurityContext.Privileged != nil {
			if *pod.Spec.Containers[i].SecurityContext.Privileged {
				fmt.Println("privileged container: ", pod.GetNamespace(), pod.GetName())
		        return &kwhmutating.MutatorResult{}, nil
			}
		}
	}

	if pod.Spec.RuntimeClassName != nil {
		fmt.Println("explicit runtime: ", pod.GetNamespace(), pod.GetName(), pod.Spec.RuntimeClassName)
        return &kwhmutating.MutatorResult{}, nil
	}

	// Mutate the pod
	fmt.Println("setting runtime to kata: ", pod.GetNamespace(), pod.GetName())

	kataRuntimeClassName := "kata"
	pod.Spec.RuntimeClassName = &kataRuntimeClassName

	return &kwhmutating.MutatorResult{
		MutatedObject: pod,
	}, nil
}

type config struct {
	certFile    string
	keyFile     string
	nsBlacklist string
	regexStr    string
}

type policy struct {
	nsBlacklist map[string]bool
}

var whPolicy *policy
var validRegex *regexp.Regexp

func initFlags() *config {
	cfg := &config{}

	fl := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fl.StringVar(&cfg.certFile, "tls-cert-file", "", "TLS certificate file")
	fl.StringVar(&cfg.keyFile, "tls-key-file", "", "TLS key file")
	fl.StringVar(&cfg.nsBlacklist, "exclude-regex-namespaces", "", "Comma separated namespace blacklist")
	fl.StringVar(&cfg.regexStr, "regex-matching-namespaces", "", "regex string, will mutate only on matching namespaces, blacklisted namespaces will be ingnored")

	fl.Parse(os.Args[1:])
	return cfg
}

func main() {
	logrusLogEntry := logrus.NewEntry(logrus.New())
	logrusLogEntry.Logger.SetLevel(logrus.DebugLevel)
	logger := kwhlogrus.NewLogrus(logrusLogEntry)

	cfg := initFlags()

	whPolicy = &policy{}
	whPolicy.nsBlacklist = make(map[string]bool)
	if cfg.nsBlacklist != "" {
		for _, s := range strings.Split(cfg.nsBlacklist, ",") {
			whPolicy.nsBlacklist[s] = true
		}
	}

	if cfg.regexStr != "" {
		// panic on invalid expression
		validRegex = regexp.MustCompile(cfg.regexStr)
	}

	// Create our mutator
	mt := kwhmutating.MutatorFunc(annotatePodMutator)

	mcfg := kwhmutating.WebhookConfig{
		ID:      "podAnnotate",
		Obj:     &corev1.Pod{},
		Mutator: mt,
		Logger:  logger,
	}
	wh, err := kwhmutating.NewWebhook(mcfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating webhook: %s", err)
		os.Exit(1)
	}

	// Get the handler for our webhook.
	whHandler, err := kwhhttp.HandlerFor(kwhhttp.HandlerConfig{Webhook: wh, Logger: logger})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating webhook handler: %s", err)
		os.Exit(1)
	}
	logger.Infof("Listening on :8080")
	err = http.ListenAndServeTLS(":8080", cfg.certFile, cfg.keyFile, whHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error serving webhook: %s", err)
		os.Exit(1)
	}
}
