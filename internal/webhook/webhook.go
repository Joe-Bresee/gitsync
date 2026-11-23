package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
)

type TriggerFunc func(context.Context, ctrl.Request) (ctrl.Result, error)

type Handler struct {
	Client  client.Client
	Trigger TriggerFunc
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "webhooks" || parts[1] != "gitsync" {
		http.NotFound(w, r)
		return
	}
	key := types.NamespacedName{Namespace: parts[2], Name: parts[3]}
	var configSync configsv1alpha1.ConfigSync
	if err := h.Client.Get(r.Context(), key, &configSync); err != nil {
		http.Error(w, "ConfigSync not found", http.StatusNotFound)
		return
	}
	if configSync.Spec.Webhook == nil || !configSync.Spec.Webhook.Enabled {
		http.Error(w, "webhook is disabled", http.StatusForbidden)
		return
	}
	if configSync.Spec.Webhook.Path != "" && r.URL.Path != configSync.Spec.Webhook.Path {
		http.NotFound(w, r)
		return
	}
	if configSync.Spec.Webhook.SecretRef != nil {
		namespace := configSync.Spec.Webhook.SecretRef.Namespace
		if namespace == "" {
			namespace = configSync.Namespace
		}
		var secret corev1.Secret
		if err := h.Client.Get(r.Context(), types.NamespacedName{Namespace: namespace, Name: configSync.Spec.Webhook.SecretRef.Name}, &secret); err != nil {
			http.Error(w, "webhook secret unavailable", http.StatusUnauthorized)
			return
		}
		token := secret.Data["token"]
		if len(token) == 0 {
			token = secret.Data["secret"]
		}
		if len(token) == 0 {
			http.Error(w, "webhook secret has no token or secret data", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read webhook body", http.StatusBadRequest)
			return
		}
		if !validSignature(r, token, body) {
			http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
			return
		}
	}
	if h.Trigger == nil {
		http.Error(w, "trigger unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err := h.Trigger(r.Context(), ctrl.Request{NamespacedName: key}); err != nil {
		http.Error(w, "reconciliation failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func validSignature(r *http.Request, secret []byte, body []byte) bool {
	provided := r.Header.Get("X-GitSync-Token")
	if provided != "" {
		return hmac.Equal([]byte(provided), secret)
	}
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return false
	}
	signature = strings.TrimPrefix(signature, "sha256=")
	decoded, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}
