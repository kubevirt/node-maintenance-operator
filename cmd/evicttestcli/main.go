package main

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	drain "k8s.io/kubectl/pkg/drain"
	nmc "kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"time"
)

const (
	DrainerTimeout = 10
)

var drainer *drain.Helper

// writer implements io.Writer interface as a pass-through for klog.
type writer struct {
	logFunc func(args ...interface{})
}

// Write passes string(p) into writer's logFunc and always returns len(p)
func (w writer) Write(p []byte) (n int, err error) {
	w.logFunc(string(p))
	return len(p), nil
}

func initDrainer() {

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Printf("can't get config error: %v\n", err)
		os.Exit(1)
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("can't init client-go %v\n", err)
		os.Exit(1)
	}
	drainer = &drain.Helper{
		Client:              cs,
		Force:               true,
		IgnoreAllDaemonSets: true,
		DisableEviction:     false,
		GracePeriodSeconds:  -1,
		// If a pod is not evicted in ``EvictionTimeSlice`` seconds, stop waiting and
		// allow it to (hopefully) complete while we process other nodes
		// Pending evictions will be checked and reattempted when the Reconcile()
		// loop gets called again
		Timeout: time.Duration(DrainerTimeout),
		OnPodDeletedOrEvicted: func(pod *corev1.Pod, usingEviction bool) {
			verbStr := "Deleted"
			if usingEviction {
				verbStr = "Evicted"
			}
			klog.Info(fmt.Sprintf("%s pod from Node", verbStr),
				"pod", fmt.Sprintf("%s/%s", pod.Name, pod.Namespace))
		},
		Out:    writer{klog.Info},
		ErrOut: writer{klog.Error},
		DryRun: false,
	}
}

func drainIt(nodeName string) {

	initDrainer()

	node, err := drainer.Client.Core().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("can't get node %s error: %v\n", nodeName, err)
	}

	err = drain.RunCordonOrUncordon(drainer, node, true)
	if err != nil {
		fmt.Printf("error while cordoning %v\n", err)
		return
	}

	list, errs := drainer.GetPodsForDeletion(nodeName)
	if errs != nil {
		err := utilerrors.NewAggregate(errs)
		fmt.Printf("failed got get pod for eviction %v\n", err)
		return
	}

	if len(list.Pods()) == 0 {
		fmt.Printf("no pods to evict\n")
		return
	}

	fmt.Printf("start evicting pods\n")

	// indicate to the user that it is evicting pods.
	if err := drainer.DeleteOrEvictPods(list.Pods()); err != nil {
		fmt.Printf("erros during eviction: %v\n", err)
		return // return original error to indicate that the call has failed.
	}
	fmt.Printf("*** eviction  completed ***\n")

}

func uncordonIt(nodeName string) {

	initDrainer() //time.Duration(timeoutInSeconds) * time.Second)

	node, err := drainer.Client.Core().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("can't get node %s error: %v\n", nodeName, err)
	}

	err = drain.RunCordonOrUncordon(drainer, node, false)
	if err != nil {
		fmt.Printf("error while cordoning %v\n", err)
		return
	}
	fmt.Printf("*** uncordon completed ***\n")
}

func deadlineInSecs(seconds int64) time.Time {
	tm := time.Now()
	tm.Add(time.Duration(seconds) * time.Second)
	return tm
}

func cleanUpExpiredPods(nodeName string, dcheck nmc.DeadlineCheck) {

	if dcheck.IsExpired() {
		fmt.Printf("cancelEviction timed out\n")
		os.Exit(1)
	}

	podList, err := nmc.GetListOfEvictedPods(drainer, nodeName, dcheck)

	if err != nil {
		fmt.Printf("cancelEviction: failed to enumerate pods in evicted state err=%v\n", err)
		os.Exit(1)
	}

	if len(podList) == 0 {
		fmt.Printf("cancelEviction: no pods in evicted state\n")
		return
	}

	if dcheck.IsExpired() {
		fmt.Printf("cancelEviction timed out after enumerting pods\n")
		os.Exit(1)
	}

	drainer.Timeout = dcheck.DurationUntilExpiration()
	drainer.DisableEviction = true
	err = drainer.DeleteOrEvictPods(podList)
	drainer.DisableEviction = false

	fmt.Printf("start deleting evicted pods, timeout: %d sec podsToDelete: %d\n", drainer.Timeout/time.Second, len(podList))

	if err != nil {
		fmt.Printf("cancelEviction: Failed to delete pods in evicted state err=%v\n", err)
		os.Exit(1)
	}
}

func cancelEviction(nodeName string, dcheck nmc.DeadlineCheck) {
	list, errs := drainer.GetPodsForDeletion(nodeName)
	if errs != nil {
		fmt.Printf("failed to get pod list %v", errs)
		return
	}

	pods := list.Pods()
	if len(pods) != 0 {

		// cancel the move
		for _, pod := range pods {
			if !dcheck.IsExpired() {
				err := drainer.Client.PolicyV1beta1().Evictions(pod.Namespace).Evict(nil)
				if err != nil {
					fmt.Printf("failed cancel eviction %v", err)
					return
				}
			}
		}
	}
}

func cancelIt(nodeName string) {

	initDrainer()

	dcheck := nmc.NewDeadlineInSeconds(3)

	cancelEviction(nodeName, dcheck)
	//cleanUpExpiredPods(nodeName, dcheck)
	fmt.Printf("finshed deleting evicted pods\n")

}

func showHelp() {

	fmt.Printf("drain <nodeName> | cancel <nodeName> | uncordon <nodeName>\n")
	os.Exit(1)
}

func main() {
	if len(os.Args) < 3 {
		showHelp()
	}

	action := os.Args[1]
	nodeName := os.Args[2]

	if action == "drain" {
		drainIt(nodeName) // * time.Second) )
	} else if action == "cancel" {
		cancelIt(nodeName)
	} else if action == "uncordon" {
		uncordonIt(nodeName)
	} else {
		showHelp()
	}
}
