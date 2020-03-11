package nodemaintenance

import (
    corev1 "k8s.io/api/core/v1"
    "time"
    "math"
)

// ContainsString checks if the string array contains the given string.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString removes the given string from the string array if exists.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return result
}

// GetPodNameList returns a list of pod names from a pod list
func GetPodNameList(pods []corev1.Pod) (result []string) {
	for _, pod := range pods {
		result = append(result, pod.ObjectMeta.Name)
	}
	return result
}

type DeadlineCheck struct {
	isSet    bool
	deadline time.Time
}

func NewDeadlineInSeconds(seconds int64) DeadlineCheck {
	tm := time.Now()
	tm.Add(time.Duration(seconds) * time.Second)

	return DeadlineCheck{isSet: true, deadline: tm}
}

func NewDeadlineAt(atTime time.Time) DeadlineCheck {
	return DeadlineCheck{isSet: true, deadline: atTime}
}

func (exp DeadlineCheck) IsExpired() bool {
	return exp.isSet && exp.deadline.After(time.Now())
}
func (exp DeadlineCheck) DurationUntilExpiration() time.Duration {
	if exp.isSet {
		return exp.deadline.Sub(time.Now())
	}
	// better not be here, better check if the deadline has been set.
	return time.Duration(math.MaxInt64)
}

