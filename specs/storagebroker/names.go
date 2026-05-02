package storagebroker

import "fmt"

const Port = 50051

func Name(clusterName string) string {
	return clusterName + "-storage-broker"
}

func URL(clusterName string) string {
	return fmt.Sprintf("http://%s:%d", Name(clusterName), Port)
}
