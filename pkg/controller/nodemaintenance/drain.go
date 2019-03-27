package nodemaintenance

import (
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/kubectl/drain"
)

func runCordonOrUncordon(r *ReconcileNodeMaintenance, node *corev1.Node, desired bool) error {
	cordonOrUncordon := "cordon"
	if !desired {
		cordonOrUncordon = "un" + cordonOrUncordon
	}

	log.Info(fmt.Sprintf("%s Node: %s", cordonOrUncordon, node.Name))

	c := drain.NewCordonHelper(node)
	if updateRequired := c.UpdateIfRequired(desired); updateRequired {
		err, patchErr := c.PatchOrReplace(r.drainer.Client)
		if patchErr != nil {
			log.Error(err, fmt.Sprintf("Unable to %s Node %s \n", cordonOrUncordon, node.Name))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func drainPods(r *ReconcileNodeMaintenance, nodeName string) error {
	list, errs := r.drainer.GetPodsForDeletion(nodeName)

	if errs != nil {
		return utilerrors.NewAggregate(errs)
	}

	if warnings := list.Warnings(); warnings != "" {
		log.Info(fmt.Sprintf("WARNING: %s\n", warnings))
	}

	if err := deleteOrEvictPods(r, list.Pods()); err != nil {
		pendingList, newErrs := r.drainer.GetPodsForDeletion(nodeName)
		log.Error(err, fmt.Sprintf("There are pending pods in node %q when an error occurred: \n", nodeName))

		for _, pendingPod := range pendingList.Pods() {
			log.Error(err, fmt.Sprintf("%s/%s\n", "pod", pendingPod.Name))
		}
		if newErrs != nil {
			log.Error(err, fmt.Sprintf("following errors also occurred:\n%s", utilerrors.NewAggregate(newErrs)))
		}
		return err
	}
	return nil
}

// deleteOrEvictPods deletes or evicts the pods on the api server
func deleteOrEvictPods(r *ReconcileNodeMaintenance, pods []corev1.Pod) error {
	if len(pods) == 0 {
		return nil
	}

	policyGroupVersion, err := drain.CheckEvictionSupport(r.drainer.Client)
	if err != nil {
		return err
	}

	getPodFn := func(namespace, name string) (*corev1.Pod, error) {
		return r.drainer.Client.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	}

	if len(policyGroupVersion) > 0 {
		return evictPods(r, pods, policyGroupVersion, getPodFn)
	}
	return deletePods(r, pods, getPodFn)

}

func deletePods(r *ReconcileNodeMaintenance, pods []corev1.Pod, getPodFn func(namespace, name string) (*corev1.Pod, error)) error {
	// 0 timeout means infinite, we use MaxInt64 to represent it.
	var globalTimeout time.Duration
	if r.drainer.Timeout == 0 {
		globalTimeout = time.Duration(math.MaxInt64)
	} else {
		globalTimeout = r.drainer.Timeout
	}
	for _, pod := range pods {
		err := r.drainer.DeletePod(pod)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	_, err := waitForDelete(pods, 1*time.Second, globalTimeout, false, getPodFn)
	return err
}

func evictPods(r *ReconcileNodeMaintenance, pods []corev1.Pod, policyGroupVersion string, getPodFn func(namespace, name string) (*corev1.Pod, error)) error {
	returnCh := make(chan error, 1)

	for _, pod := range pods {
		go func(pod corev1.Pod, returnCh chan error) {
			for {
				log.Info(fmt.Sprintf("evicting pod %q\n", pod.Name))
				err := r.drainer.EvictPod(pod, policyGroupVersion)
				if err == nil {
					break
				} else if apierrors.IsNotFound(err) {
					returnCh <- nil
					return
				} else if apierrors.IsTooManyRequests(err) {
					log.Error(err, fmt.Sprintf("error when evicting pod %q (will retry after 5s)\n", pod.Name))
					time.Sleep(5 * time.Second)
				} else {
					returnCh <- fmt.Errorf("error when evicting pod %q: %v", pod.Name, err)
					return
				}
			}
			_, err := waitForDelete([]corev1.Pod{pod}, 1*time.Second, time.Duration(math.MaxInt64), true, getPodFn)
			if err == nil {
				returnCh <- nil
			} else {
				returnCh <- fmt.Errorf("error when waiting for pod %q terminating: %v", pod.Name, err)
			}
		}(pod, returnCh)
	}

	doneCount := 0
	var errors []error

	// 0 timeout means infinite, we use MaxInt64 to represent it.
	var globalTimeout time.Duration
	if r.drainer.Timeout == 0 {
		globalTimeout = time.Duration(math.MaxInt64)
	} else {
		globalTimeout = r.drainer.Timeout
	}
	globalTimeoutCh := time.After(globalTimeout)
	numPods := len(pods)
	for doneCount < numPods {
		select {
		case err := <-returnCh:
			doneCount++
			if err != nil {
				errors = append(errors, err)
			}
		case <-globalTimeoutCh:
			return fmt.Errorf("drain did not complete within %v", globalTimeout)
		}
	}
	return utilerrors.NewAggregate(errors)
}

func waitForDelete(pods []corev1.Pod, interval, timeout time.Duration, usingEviction bool, getPodFn func(string, string) (*corev1.Pod, error)) ([]corev1.Pod, error) {
	var verbStr string
	if usingEviction {
		verbStr = "evicted"
	} else {
		verbStr = "deleted"
	}

	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		pendingPods := []corev1.Pod{}
		for i, pod := range pods {
			p, err := getPodFn(pod.Namespace, pod.Name)
			if apierrors.IsNotFound(err) || (p != nil && p.ObjectMeta.UID != pod.ObjectMeta.UID) {
				log.Info(fmt.Sprintf("%s Pod: %s", verbStr, pod.Name))
				continue
			} else if err != nil {
				return false, err
			} else {
				pendingPods = append(pendingPods, pods[i])
			}
		}
		pods = pendingPods
		if len(pendingPods) > 0 {
			return false, nil
		}
		return true, nil
	})
	return pods, err
}
