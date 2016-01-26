package apifaker

import (
	"encoding/json"
	"fmt"
	"github.com/Focinfi/gset"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"sync"
)

var ColumnCountError = fmt.Errorf("Has wrong count of columns")
var ColumnNameError = fmt.Errorf("Has wrong column")

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Model struct {
	Name string `json:"resource_name"`
	// Seeds contains value of "seeds" in json files
	Seeds   []map[string]interface{} `json:"seeds"`
	Columns []Column                 `json:"columns"`

	// Set Compose *gset.SetThreadSafe to contains data
	Set *gset.SetThreadSafe `json:"-"`

	// filePath json file path
	filePath string `json:"-"`
	// currentId record the number of times of adding
	currentId int `json:"-"`
	sync.Mutex
}

// NewModel allocates and returns a new Model
func NewModel() *Model {
	return &Model{
		Seeds:   []map[string]interface{}{},
		Columns: []Column{},
		Set:     gset.NewSetThreadSafe(),
	}
}

// nextId plus 1 to Model.currentId return return it
func (model *Model) nextId() int {
	model.currentId++
	return model.currentId
}

// Has return if m has LineItem with id param
func (model *Model) Has(id int) bool {
	return model.Set.Has(gset.T(id))
}

// Get get and return element with id param, also return
// if it's not nil or a LineItem
func (model Model) Get(id int) (li LineItem, ok bool) {
	var element interface{}
	if element, ok = model.Set.Get(id); ok {
		li, ok = element.(LineItem)
	}
	return
}

// Add add a LineItem to Model.Set
func (model *Model) Add(li LineItem) error {
	model.Lock()
	defer model.Unlock()

	if err := model.checkSeed(li.ToMap()); err != nil {
		return err
	} else {
		li.Set("id", model.nextId())
		model.Set.Add(li)
	}

	return nil
}

// Update a LineItem in Model
func (model *Model) Update(id int, li *LineItem) error {
	if err := model.checkSeed(li.dataMap); err != nil {
		return err
	} else {
		if model.Set.Has(gset.T(id)) {
			li.Set("id", id)
			model.Set.Add(li)
		} else {
			return fmt.Errorf("model[id:%d] does not exsit", id)
		}
	}

	return nil
}

// Delete
func (m *Model) Delete(id int) {
	m.Set.Remove(gset.T(id))
}

// ToLineItems allocate a new LineItems filled with
// Model elements slice
func (m Model) ToLineItems() LineItems {
	lis := []LineItem{}
	models := m.Set.ToSlice()
	for _, model := range models {
		if li, ok := model.(LineItem); ok {
			lis = append(lis[:], li)
		}
	}
	return LineItems(lis)
}

// UpdateWithAttrsInGinContext find a LineItem with id param,
// update it with attrs from gin.Contex.PostForm(),
// returns status in net/http package and object for response
func (model *Model) UpdateWithAttrsInGinContext(id int, ctx *gin.Context) (int, interface{}) {
	// check if element does exsit
	li, ok := model.Get(id)
	if !ok {
		return http.StatusNotFound, nil
	}

	// update model
	for _, column := range model.Columns {
		if value := ctx.PostForm(column.Name); value != "" {
			// TODO: check type
			li.Set(column.Name, value)
		}
	}
	return http.StatusOK, li.ToMap()
}

// UpdateWithAllAttrsInGinContex find a LineItem with id param,
// allocate a new LineItem with attrs from gin.Context.PostForm(),
// replace this LineItem with the new one,
// returns status in net/http package, and object for response
func (model *Model) UpdateWithAllAttrsInGinContex(id int, ctx *gin.Context) (int, interface{}) {
	// check if element does exsit
	_, ok := model.Get(id)
	if !ok {
		return http.StatusNotFound, nil
	}

	// create a new item
	newItem, err := NewLineItemWithGinContext(ctx, model)
	// update item
	model.Update(id, &newItem)

	if err != nil {
		return http.StatusBadRequest, map[string]string{"message": err.Error()}
	} else {
		return http.StatusOK, newItem.ToMap()
	}
}

// checkSeed check specific seed
func (model *Model) checkSeed(seed map[string]interface{}) error {
	columns := model.Columns
	delete(seed, "id")

	if len(seed) != len(columns) {
		return ColumnCountError
	}

	for _, column := range columns {
		if _, ok := seed[column.Name]; !ok {
			ColumnNameError = fmt.Errorf("has wrong column: %s", column.Name)
			return ColumnNameError
		}
		// TODO: check type
	}

	return nil
}

// checkSeeds() check if every item of this Model object's Seeds
// is in line of description of its Columns.
func (model *Model) checkSeeds() error {
	for _, seed := range model.Seeds {
		if err := model.checkSeed(seed); err != nil {
			return err
		}
	}

	return nil
}

// setItems
func (model *Model) setItems() {
	if model.Set == nil {
		model.Set = gset.NewSetThreadSafe()
	}
	for _, seed := range model.Seeds {
		model.Add(NewLineItemWithMap(seed))
	}
}

func (model *Model) SetToSeeds() {
	models := model.ToLineItems()
	sort.Sort(models)
	model.Seeds = models.ToSlice()
}

func (model *Model) SaveToFile(path string) error {
	// open file
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// set m.Set to m.seeds
	model.SetToSeeds()
	bytes, err := json.Marshal(model)
	if err != nil {
		return err
	}
	_, err = file.WriteString(string(bytes))
	return err
}

// NewModelWithPath allocates and returns a new Model,
// using the given path as it's json file path
func NewModelWithPath(path string) (*Model, error) {
	// open file
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// read and unmarshal file
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	model := NewModel()
	model.filePath = path
	if err = json.Unmarshal(bytes, model); err != nil {
		err = fmt.Errorf("[apifaker] json format error: %s, file: %s", err.Error(), path)
	} else {
		// check Seeds
		err = model.checkSeeds()

		// set items
		if err == nil {
			model.setItems()
		}
	}

	return model, err
}