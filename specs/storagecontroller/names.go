package storagecontroller

import "fmt"

const Port = 8080

func Name(clusterName string) string {
	return clusterName + "-storage-controller"
}

func URL(clusterName string) string {
	return fmt.Sprintf("http://%s:%d", Name(clusterName), Port)
}
