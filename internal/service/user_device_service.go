package service

import (
	"encoding/json"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

type UserDeviceService struct{}

// var UserDeviceService userDeviceService
func NewDeviceService() *UserDeviceService {
	return &UserDeviceService{}
}
func (*UserDeviceService) CreateOrUpdate(req *vai.RequestHeader, userID, openID string) (*model.UserDeviceInfo, error) {
	device := req.GetDevice()
	os := constants.MappingOS(device.GetOs())
	idfv := device.GetIdfv()
	idfa := device.GetIdfa()
	andrioID := device.GetAndroidId()
	oaid := device.GetOaid()
	appStore := req.GetAppStore().String()

	deviceJson, err := json.Marshal(device)
	if err != nil {
		return nil, err
	}

	info := model.UserDeviceInfo{
		UserID:         userID,
		OS:             os,
		IDFA:           idfa,
		IDFV:           idfv,
		AndroidID:      andrioID,
		OAID:           oaid,
		OpenID:         openID,
		AppStore:       appStore,
		DeviceInfoJson: string(deviceJson),
	}
	uddao := dao.NewUserDeviceDao()
	return uddao.CreateOrUpdate(info)
}
