package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"clawreef/internal/repository"
	"clawreef/internal/services/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecoveryService handles automatic recovery of Failed Pods after node restart
type RecoveryService struct {
	instanceRepo    repository.InstanceRepository
	instanceService InstanceService
	podService      *k8s.PodService
	k8sClient       *k8s.Client
	interval        time.Duration
	stopChan        chan struct{}
}

// NewRecoveryService creates a new recovery service
func NewRecoveryService(
	instanceRepo repository.InstanceRepository,
	instanceService InstanceService,
	k8sClient *k8s.Client,
) *RecoveryService {
	return &RecoveryService{
		instanceRepo:    instanceRepo,
		instanceService: instanceService,
		podService:      k8s.NewPodService(),
		k8sClient:       k8sClient,
		interval:        30 * time.Second, // Check every 30 seconds
		stopChan:        make(chan struct{}),
	}
}

// Start starts the recovery service
func (s *RecoveryService) Start() {
	fmt.Println("[RecoveryService] Starting automatic pod recovery service...")
	go s.reconcileLoop()
}

// Stop stops the recovery service
func (s *RecoveryService) Stop() {
	close(s.stopChan)
}

// reconcileLoop runs the recovery check loop
func (s *RecoveryService) reconcileLoop() {
	fmt.Printf("[RecoveryService] Reconcile loop started, interval=%v\n", s.interval)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.reconcileFailedPods()
		case <-s.stopChan:
			fmt.Println("[RecoveryService] Stopping recovery service...")
			return
		}
	}
}

// reconcileFailedPods scans all namespaces for Failed Pods and recovers them
func (s *RecoveryService) reconcileFailedPods() {
	if s.k8sClient == nil {
		return
	}

	ctx := context.Background()

	// Find all clawreef namespaces
	namespaces, err := s.findClawreefNamespaces(ctx)
	if err != nil {
		fmt.Printf("[RecoveryService] Error finding namespaces: %v\n", err)
		return
	}

	if len(namespaces) == 0 {
		return
	}

	fmt.Printf("[RecoveryService] Scanning %d namespace(s) for failed pods...\n", len(namespaces))

	for _, ns := range namespaces {
		s.reconcileNamespace(ctx, ns)
	}
}

// reconcileNamespace checks a single namespace for failed pods
func (s *RecoveryService) reconcileNamespace(ctx context.Context, namespace string) {
	// List all pods in this namespace with "managed-by=clawreef" label
	pods, err := s.k8sClient.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "managed-by=clawreef",
	})
	if err != nil {
		fmt.Printf("[RecoveryService] Error listing pods in namespace %s: %v\n", namespace, err)
		return
	}

	for _, pod := range pods.Items {
		// Only process Failed pods
		if pod.Status.Phase != corev1.PodFailed && 
       		   pod.Status.Phase != corev1.PodUnknown {
			continue
		}


		instanceIDStr := pod.Labels["instance-id"]
		if instanceIDStr == "" {
			continue
		}

		instanceID, err := strconv.Atoi(instanceIDStr)
		if err != nil {
			fmt.Printf("[RecoveryService] Invalid instance-id label on pod %s: %s\n", pod.Name, instanceIDStr)
			continue
		}

		s.recoverPod(ctx, &pod, instanceID)
	}
}

// recoverPod attempts to recover a single failed pod
func (s *RecoveryService) recoverPod(ctx context.Context, pod *corev1.Pod, instanceID int) {
	// Check instance DB status
	instance, err := s.instanceRepo.GetByID(instanceID)
	if err != nil {
		fmt.Printf("[RecoveryService] Error getting instance %d from DB: %v\n", instanceID, err)
		return
	}
	if instance == nil {
		// Instance doesn't exist in DB, just delete the orphan pod
		fmt.Printf("[RecoveryService] Instance %d not found in DB, deleting orphan pod %s\n", instanceID, pod.Name)
		s.deleteOrphanPod(ctx, pod)
		return
	}

	// Only auto-recover instances that should be running
	if instance.Status != "running" && instance.Status != "error" {
		fmt.Printf("[RecoveryService] Instance %d status is '%s', skipping recovery (pod %s is failed)\n",
			instanceID, instance.Status, pod.Name)
		return
	}

	fmt.Printf("[RecoveryService] Instance %d is marked as '%s' but pod %s is Failed — initiating recovery\n",
		instanceID, instance.Status, pod.Name)

	// Step 1: Mark instance as "stopped" so Start() will accept the request
	instance.Status = "stopped"
	instance.UpdatedAt = time.Now()
	if err := s.instanceRepo.Update(instance); err != nil {
		fmt.Printf("[RecoveryService] Failed to update instance %d status to 'stopped': %v\n", instanceID, err)
		return
	}

	// Step 2: Delete the failed pod
	fmt.Printf("[RecoveryService] Deleting failed pod %s in namespace %s\n", pod.Name, pod.Namespace)
	if err := s.k8sClient.Clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		fmt.Printf("[RecoveryService] Failed to delete pod %s: %v\n", pod.Name, err)
		// Rollback: restore instance status
		instance.Status = "running"
		instance.UpdatedAt = time.Now()
		s.instanceRepo.Update(instance)
		return
	}

	// Step 3: Wait briefly for pod deletion to complete
	time.Sleep(2 * time.Second)

	// Step 4: Call Start() to recreate the pod
	fmt.Printf("[RecoveryService] Calling Start() for instance %d\n", instanceID)
	if err := s.instanceService.Start(instanceID); err != nil {
		fmt.Printf("[RecoveryService] Start() failed for instance %d: %v\n", instanceID, err)
		// Don't rollback status here — let next reconcile cycle handle it
		return
	}

	fmt.Printf("[RecoveryService] Successfully initiated recovery for instance %d\n", instanceID)
}

// findClawreefNamespaces finds all namespaces managed by clawreef
func (s *RecoveryService) findClawreefNamespaces(ctx context.Context) ([]string, error) {
	nsList, err := s.k8sClient.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "managed-by=clawreef",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
}

// deleteOrphanPod deletes a pod whose instance no longer exists in DB
func (s *RecoveryService) deleteOrphanPod(ctx context.Context, pod *corev1.Pod) {
	if err := s.k8sClient.Clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		fmt.Printf("[RecoveryService] Failed to delete orphan pod %s: %v\n", pod.Name, err)
	} else {
		fmt.Printf("[RecoveryService] Deleted orphan pod %s\n", pod.Name)
	}
}
