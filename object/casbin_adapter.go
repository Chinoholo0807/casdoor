// Copyright 2022 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package object

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casdoor/casdoor/util"
	xormadapter "github.com/casdoor/xorm-adapter/v3"
	"github.com/xorm-io/core"
)

type CasbinAdapter struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`

	Type  string `xorm:"varchar(100)" json:"type"`
	Model string `xorm:"varchar(100)" json:"model"`

	Host         string `xorm:"varchar(100)" json:"host"`
	Port         int    `json:"port"`
	User         string `xorm:"varchar(100)" json:"user"`
	Password     string `xorm:"varchar(100)" json:"password"`
	DatabaseType string `xorm:"varchar(100)" json:"databaseType"`
	Database     string `xorm:"varchar(100)" json:"database"`
	Table        string `xorm:"varchar(100)" json:"table"`
	IsEnabled    bool   `json:"isEnabled"`

	Adapter *xormadapter.Adapter `xorm:"-" json:"-"`
}

func GetCasbinAdapterCount(owner, field, value string) (int64, error) {
	session := GetSession(owner, -1, -1, field, value, "", "")
	return session.Count(&CasbinAdapter{})
}

func GetCasbinAdapters(owner string) ([]*CasbinAdapter, error) {
	adapters := []*CasbinAdapter{}
	err := adapter.Engine.Desc("created_time").Find(&adapters, &CasbinAdapter{Owner: owner})
	if err != nil {
		return adapters, err
	}

	return adapters, nil
}

func GetPaginationCasbinAdapters(owner string, offset, limit int, field, value, sortField, sortOrder string) ([]*CasbinAdapter, error) {
	adapters := []*CasbinAdapter{}
	session := GetSession(owner, offset, limit, field, value, sortField, sortOrder)
	err := session.Find(&adapters)
	if err != nil {
		return adapters, err
	}

	return adapters, nil
}

func getCasbinAdapter(owner, name string) (*CasbinAdapter, error) {
	if owner == "" || name == "" {
		return nil, nil
	}

	casbinAdapter := CasbinAdapter{Owner: owner, Name: name}
	existed, err := adapter.Engine.Get(&casbinAdapter)
	if err != nil {
		return nil, err
	}

	if existed {
		return &casbinAdapter, nil
	} else {
		return nil, nil
	}
}

func GetCasbinAdapter(id string) (*CasbinAdapter, error) {
	owner, name := util.GetOwnerAndNameFromId(id)
	return getCasbinAdapter(owner, name)
}

func UpdateCasbinAdapter(id string, casbinAdapter *CasbinAdapter) (bool, error) {
	owner, name := util.GetOwnerAndNameFromId(id)
	if casbinAdapter, err := getCasbinAdapter(owner, name); casbinAdapter == nil {
		return false, err
	}

	session := adapter.Engine.ID(core.PK{owner, name}).AllCols()
	if casbinAdapter.Password == "***" {
		session.Omit("password")
	}
	affected, err := session.Update(casbinAdapter)
	if err != nil {
		return false, err
	}

	return affected != 0, nil
}

func AddCasbinAdapter(casbinAdapter *CasbinAdapter) (bool, error) {
	affected, err := adapter.Engine.Insert(casbinAdapter)
	if err != nil {
		return false, err
	}

	return affected != 0, nil
}

func DeleteCasbinAdapter(casbinAdapter *CasbinAdapter) (bool, error) {
	affected, err := adapter.Engine.ID(core.PK{casbinAdapter.Owner, casbinAdapter.Name}).Delete(&CasbinAdapter{})
	if err != nil {
		return false, err
	}

	return affected != 0, nil
}

func (casbinAdapter *CasbinAdapter) GetId() string {
	return fmt.Sprintf("%s/%s", casbinAdapter.Owner, casbinAdapter.Name)
}

func (casbinAdapter *CasbinAdapter) getTable() string {
	if casbinAdapter.DatabaseType == "mssql" {
		return fmt.Sprintf("[%s]", casbinAdapter.Table)
	} else {
		return casbinAdapter.Table
	}
}

func initEnforcer(modelObj *Model, casbinAdapter *CasbinAdapter) (*casbin.Enforcer, error) {
	// init Adapter
	if casbinAdapter.Adapter == nil {
		var dataSourceName string
		if casbinAdapter.DatabaseType == "mssql" {
			dataSourceName = fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s", casbinAdapter.User, casbinAdapter.Password, casbinAdapter.Host, casbinAdapter.Port, casbinAdapter.Database)
		} else if casbinAdapter.DatabaseType == "postgres" {
			dataSourceName = fmt.Sprintf("user=%s password=%s host=%s port=%d sslmode=disable dbname=%s", casbinAdapter.User, casbinAdapter.Password, casbinAdapter.Host, casbinAdapter.Port, casbinAdapter.Database)
		} else {
			dataSourceName = fmt.Sprintf("%s:%s@tcp(%s:%d)/", casbinAdapter.User, casbinAdapter.Password, casbinAdapter.Host, casbinAdapter.Port)
		}

		if !isCloudIntranet {
			dataSourceName = strings.ReplaceAll(dataSourceName, "dbi.", "db.")
		}

		var err error
		casbinAdapter.Adapter, err = xormadapter.NewAdapterByEngineWithTableName(NewAdapter(casbinAdapter.DatabaseType, dataSourceName, casbinAdapter.Database).Engine, casbinAdapter.getTable(), "")
		if err != nil {
			return nil, err
		}
	}

	// init Model
	m, err := model.NewModelFromString(modelObj.ModelText)
	if err != nil {
		return nil, err
	}

	// init Enforcer
	enforcer, err := casbin.NewEnforcer(m, casbinAdapter.Adapter)
	if err != nil {
		return nil, err
	}

	return enforcer, nil
}

func safeReturn(policy []string, i int) string {
	if len(policy) > i {
		return policy[i]
	} else {
		return ""
	}
}

func matrixToCasbinRules(Ptype string, policies [][]string) []*xormadapter.CasbinRule {
	res := []*xormadapter.CasbinRule{}

	for _, policy := range policies {
		line := xormadapter.CasbinRule{
			Ptype: Ptype,
			V0:    safeReturn(policy, 0),
			V1:    safeReturn(policy, 1),
			V2:    safeReturn(policy, 2),
			V3:    safeReturn(policy, 3),
			V4:    safeReturn(policy, 4),
			V5:    safeReturn(policy, 5),
		}
		res = append(res, &line)
	}

	return res
}

func SyncPolicies(casbinAdapter *CasbinAdapter) ([]*xormadapter.CasbinRule, error) {
	modelObj, err := getModel(casbinAdapter.Owner, casbinAdapter.Model)
	if err != nil {
		return nil, err
	}

	if modelObj == nil {
		return nil, fmt.Errorf("The model: %s does not exist", util.GetId(casbinAdapter.Owner, casbinAdapter.Model))
	}

	enforcer, err := initEnforcer(modelObj, casbinAdapter)
	if err != nil {
		return nil, err
	}

	policies := matrixToCasbinRules("p", enforcer.GetPolicy())
	if strings.Contains(modelObj.ModelText, "[role_definition]") {
		policies = append(policies, matrixToCasbinRules("g", enforcer.GetGroupingPolicy())...)
	}

	return policies, nil
}

func UpdatePolicy(oldPolicy, newPolicy []string, casbinAdapter *CasbinAdapter) (bool, error) {
	modelObj, err := getModel(casbinAdapter.Owner, casbinAdapter.Model)
	if err != nil {
		return false, err
	}

	enforcer, err := initEnforcer(modelObj, casbinAdapter)
	if err != nil {
		return false, err
	}

	affected, err := enforcer.UpdatePolicy(oldPolicy, newPolicy)
	if err != nil {
		return affected, err
	}
	return affected, nil
}

func AddPolicy(policy []string, casbinAdapter *CasbinAdapter) (bool, error) {
	modelObj, err := getModel(casbinAdapter.Owner, casbinAdapter.Model)
	if err != nil {
		return false, err
	}

	enforcer, err := initEnforcer(modelObj, casbinAdapter)
	if err != nil {
		return false, err
	}

	affected, err := enforcer.AddPolicy(policy)
	if err != nil {
		return affected, err
	}
	return affected, nil
}

func RemovePolicy(policy []string, casbinAdapter *CasbinAdapter) (bool, error) {
	modelObj, err := getModel(casbinAdapter.Owner, casbinAdapter.Model)
	if err != nil {
		return false, err
	}

	enforcer, err := initEnforcer(modelObj, casbinAdapter)
	if err != nil {
		return false, err
	}

	affected, err := enforcer.RemovePolicy(policy)
	if err != nil {
		return affected, err
	}

	return affected, nil
}
