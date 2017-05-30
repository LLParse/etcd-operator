package ranchutil

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	rancher "github.com/rancher/go-rancher/v2"
)

const (
	storageClassPrefix        = "etcd-backup"
	BackupPodSelectorAppField = "etcd_backup_tool"
	backupPVVolName           = "etcd-backup-storage"
	awsCredentialDir          = "/root/.aws/"
	awsConfigDir              = "/root/.aws/config/"
	awsSecretVolName          = "secret-aws"
	awsConfigVolName          = "config-aws"
	fromDirMountDir           = "/mnt/backup/from"

	PVBackupV1 = "v1" // TODO: refactor and combine this with pkg/backup.PVBackupV1
)

func CreateVolume(client *rancher.RancherClient, name, driver string) error {
	_, err := client.Volume.Create(&rancher.Volume{
		Name:       name,
		Driver:     driver,
		DriverOpts: map[string]interface{}{},
	})
	return err
}

func CopyVolume(client *rancher.RancherClient, fromClusterName, toClusterName string) error {
	from := path.Join(fromDirMountDir, PVBackupV1)
	to := path.Join(constants.BackupMountDir, PVBackupV1)

	c := &rancher.Container{
		Name: copyVolumePodName(toClusterName),
		Labels: map[string]interface{}{
			"etcd_cluster": toClusterName,
		},
		ImageUuid: "docker:alpine:latest",
		Command: []string{
			"/bin/sh",
			"-ec",
			fmt.Sprintf("mkdir -p %[2]s; cp -r %[1]s/* %[2]s/", from, to),
		},
		DataVolumes: []string{
			strings.Join([]string{fromClusterName, fromDirMountDir, "ro"}, ":"),
			strings.Join([]string{toClusterName, constants.BackupMountDir}, ":"),
		},
	}

	var err error
	if c, err = client.Container.Create(c); err != nil {
		return err
	}

	err = retryutil.Retry(10*time.Second, 12, func() (bool, error) {
		cn, err2 := client.Container.ById(c.Id)
		if err2 != nil {
			return false, err2
		}
		switch cn.State {
		// success
		case "stopped":
			return true, nil
			// TODO: detect failure somehow
			// default:
			// return false, fmt.Errorf("backup copy container (%s) failed: %v", cn.Name, cn.Error)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup copy container (%s) to succeed: %v", c.Name, err)
	}
	return client.Container.Delete(c)
}

func copyVolumePodName(clusterName string) string {
	return clusterName + "-copyvolume"
}
