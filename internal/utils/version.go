package utils

import (
	"github.com/Masterminds/semver/v3"
)

// 检查版本是否在指定范围内
func CheckVersionRange(versionStr, constraintStr string) (bool, error) {
	v, err := semver.NewVersion(versionStr)
	if err != nil {
		return false, err
	}

	c, err := semver.NewConstraint(constraintStr)
	if err != nil {
		return false, err
	}

	return c.Check(v), nil
}
