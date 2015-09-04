package auctioneer

import (
	"errors"

	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/rep"
)

type TaskStartRequest struct {
	rep.Task
}

func NewTaskStartRequest(task rep.Task) TaskStartRequest {
	return TaskStartRequest{task}
}

func NewTaskStartRequestFromModel(t *models.Task) TaskStartRequest {
	return TaskStartRequest{rep.NewTask(t.TaskGuid, rep.NewResource(t.MemoryMb, t.DiskMb, t.RootFs))}
}

type LRPStartRequest struct {
	ProcessGuid string `json:"process_guid"`
	rep.Resource
	Indices []int `json:"indices"`
}

func NewLRPStartRequest(processGuid string, indices []int, res rep.Resource) LRPStartRequest {
	return LRPStartRequest{
		ProcessGuid: processGuid,
		Indices:     indices,
		Resource:    res,
	}
}

func NewLRPStartRequestFromModel(d *models.DesiredLRP, indices ...int) LRPStartRequest {
	return NewLRPStartRequest(d.ProcessGuid, indices, rep.NewResource(d.MemoryMb, d.DiskMb, d.RootFs))
}

func (lrpstart *LRPStartRequest) Validate() error {
	switch {
	case lrpstart.ProcessGuid == "":
		return errors.New("proccess guid is empty")
	case len(lrpstart.Indices) == 0:
		return errors.New("indices must not be empty")
	case lrpstart.Resource.Empty():
		return errors.New("resources cannot be empty")
	default:
		return nil
	}
}
