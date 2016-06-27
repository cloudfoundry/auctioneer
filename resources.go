package auctioneer

import (
	"errors"

	"code.cloudfoundry.org/bbs/models"
	"github.com/cloudfoundry-incubator/rep"
)

type TaskStartRequest struct {
	rep.Task
}

func NewTaskStartRequest(task rep.Task) TaskStartRequest {
	return TaskStartRequest{task}
}

func NewTaskStartRequestFromModel(taskGuid, domain string, taskDef *models.TaskDefinition) TaskStartRequest {
	volumeMounts := []string{}
	for _, volumeMount := range taskDef.VolumeMounts {
		volumeMounts = append(volumeMounts, volumeMount.Driver)
	}
	return TaskStartRequest{rep.NewTask(taskGuid, domain, rep.NewResource(taskDef.MemoryMb, taskDef.DiskMb, taskDef.RootFs, volumeMounts))}
}

func (t *TaskStartRequest) Validate() error {
	switch {
	case t.TaskGuid == "":
		return errors.New("task guid is empty")
	case t.Resource.Empty():
		return errors.New("resources cannot be empty")
	default:
		return nil
	}
}

type LRPStartRequest struct {
	ProcessGuid string `json:"process_guid"`
	Domain      string `json:"domain"`
	Indices     []int  `json:"indices"`
	rep.Resource
}

func NewLRPStartRequest(processGuid, domain string, indices []int, res rep.Resource) LRPStartRequest {
	return LRPStartRequest{
		ProcessGuid: processGuid,
		Domain:      domain,
		Indices:     indices,
		Resource:    res,
	}
}

func NewLRPStartRequestFromModel(d *models.DesiredLRP, indices ...int) LRPStartRequest {
	volumeDrivers := []string{}
	for _, volumeMount := range d.VolumeMounts {
		volumeDrivers = append(volumeDrivers, volumeMount.Driver)
	}

	return NewLRPStartRequest(d.ProcessGuid, d.Domain, indices, rep.NewResource(d.MemoryMb, d.DiskMb, d.RootFs, volumeDrivers))
}

func NewLRPStartRequestFromSchedulingInfo(s *models.DesiredLRPSchedulingInfo, indices ...int) LRPStartRequest {
	return NewLRPStartRequest(s.ProcessGuid, s.Domain, indices, rep.NewResource(s.MemoryMb, s.DiskMb, s.RootFs, s.VolumePlacement.DriverNames))
}

func (lrpstart *LRPStartRequest) Validate() error {
	switch {
	case lrpstart.ProcessGuid == "":
		return errors.New("proccess guid is empty")
	case lrpstart.Domain == "":
		return errors.New("domain is empty")
	case len(lrpstart.Indices) == 0:
		return errors.New("indices must not be empty")
	case lrpstart.Resource.Empty():
		return errors.New("resources cannot be empty")
	default:
		return nil
	}
}
