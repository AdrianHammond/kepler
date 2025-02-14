/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
lr.go
estimate (node/pod) component and total power by linear regression approach when trained model weights are available.
The model weights can be obtained by Kepler Model Server or configured initial model URL.
*/

package local

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/sustainable-computing-io/kepler/pkg/config"
	"github.com/sustainable-computing-io/kepler/pkg/model/types"
)

var (
	SampleDynPowerValue float64 = 100.0

	containerFeatureNames = []string{
		config.CPUCycle,
		config.CPUInstruction,
		config.CacheMiss,
		config.CgroupfsMemory,
		config.CgroupfsKernelMemory,
		config.CgroupfsTCPMemory,
		config.CgroupfsCPU,
		config.CgroupfsSystemCPU,
		config.CgroupfsUserCPU,
		config.CgroupfsReadIO,
		config.CgroupfsWriteIO,
		config.BlockDevicesIO,
		config.KubeletContainerCPU,
		config.KubeletContainerMemory,
		config.KubeletNodeCPU,
		config.KubeletNodeMemory,
	}
	systemMetaDataFeatureNames = []string{"cpu_architecture"}
	containerFeatureValues     = [][]float64{
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, // container A
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, // container B
	}
	nodeFeatureValues           = []float64{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
	systemMetaDataFeatureValues = []string{"Sandy Bridge"}
)

var (
	SampleCategoricalFeatures = map[string]CategoricalFeature{
		"Sandy Bridge": {
			Weight: 1.0,
		},
	}
	SampleCoreNumericalVars = map[string]NormalizedNumericalFeature{
		"cpu_cycles": {Weight: 1.0, Mean: 0, Variance: 1},
	}
	SampleDramNumbericalVars = map[string]NormalizedNumericalFeature{
		"cache_miss": {Weight: 1.0, Mean: 0, Variance: 1},
	}
	SampleComponentWeightResponse = ComponentModelWeights{
		"core": genWeights(SampleCoreNumericalVars),
		"dram": genWeights(SampleDramNumbericalVars),
	}
	SamplePowerWeightResponse = genWeights(SampleCoreNumericalVars)
)

func genWeights(numericalVars map[string]NormalizedNumericalFeature) ModelWeights {
	return ModelWeights{
		AllWeights{
			BiasWeight:           1.0,
			CategoricalVariables: map[string]map[string]CategoricalFeature{"cpu_architecture": SampleCategoricalFeatures},
			NumericalVariables:   numericalVars,
		},
	}
}

func getDummyWeights(w http.ResponseWriter, r *http.Request) {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	var req ModelRequest
	err = json.Unmarshal(reqBody, &req)
	if err != nil {
		panic(err)
	}
	if strings.Contains(req.OutputType, "ComponentModelWeight") {
		err = json.NewEncoder(w).Encode(SampleComponentWeightResponse)
	} else {
		err = json.NewEncoder(w).Encode(SamplePowerWeightResponse)
	}
	if err != nil {
		panic(err)
	}
}

func genLinearRegressor(outputType types.ModelOutputType, modelServerEndpoint, modelWeightsURL string) LinearRegressor {
	config.ModelServerEnable = true
	config.ModelServerEndpoint = modelServerEndpoint
	return LinearRegressor{
		ModelServerEndpoint:         modelServerEndpoint,
		OutputType:                  outputType,
		FloatFeatureNames:           containerFeatureNames,
		SystemMetaDataFeatureNames:  systemMetaDataFeatureNames,
		SystemMetaDataFeatureValues: systemMetaDataFeatureValues,
		ModelWeightsURL:             modelWeightsURL,
	}
}

var _ = Describe("Test LR Weight Unit", func() {
	Context("with dummy model server", func() {
		It("Get Node Platform Power By Linear Regression with ModelServerEndpoint", func() {
			testServer := httptest.NewServer(http.HandlerFunc(getDummyWeights))
			r := genLinearRegressor(types.AbsModelWeight, testServer.URL, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
			powers, err := r.GetPlatformPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(powers)).Should(Equal(1))
			// TODO: verify if the power makes sense
			Expect(powers[0]).Should(BeEquivalentTo(4))
		})

		It("Get Node Components Power By Linear Regression Estimator with ModelServerEndpoint", func() {
			testServer := httptest.NewServer(http.HandlerFunc(getDummyWeights))
			r := genLinearRegressor(types.AbsComponentModelWeight, testServer.URL, "ComponentModelWeight")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
			compPowers, err := r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(compPowers)).Should(Equal(1))
			// TODO: verify if the power makes sense
			Expect(compPowers[0].Core).Should(BeEquivalentTo(4000))
		})

		It("Get Container Platform Power By Linear Regression Estimator with ModelServerEndpoint", func() {
			testServer := httptest.NewServer(http.HandlerFunc(getDummyWeights))
			r := genLinearRegressor(types.DynModelWeight, testServer.URL, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, containerFeatureValues := range containerFeatureValues {
				r.AddContainerFeatureValues(containerFeatureValues) // add samples to the power model
			}
			powers, err := r.GetPlatformPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(powers)).Should(Equal(len(containerFeatureValues)))
			// TODO: verify if the power makes sense
			Expect(powers[0]).Should(BeEquivalentTo(3))
		})

		It("Get Container Components Power By Linear Regression Estimator with ModelServerEndpoint", func() {
			testServer := httptest.NewServer(http.HandlerFunc(getDummyWeights))
			r := genLinearRegressor(types.DynComponentModelWeight, testServer.URL, "ComponentModelWeight")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, containerFeatureValues := range containerFeatureValues {
				r.AddContainerFeatureValues(containerFeatureValues) // add samples to the power model
			}
			compPowers, err := r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(compPowers)).Should(Equal(len(containerFeatureValues)))
			// TODO: verify if the power makes sense
			Expect(compPowers[0].Core).Should(BeEquivalentTo(3000))
		})
	})

	Context("without model server", func() {
		It("Get Node Components Power By Linear Regression Estimator without ModelServerEndpoint", func() {
			/// Estimate Node Components Absolute Power using Linear Regression
			initModelURL := "https://raw.githubusercontent.com/sustainable-computing-io/kepler-model-server/main/tests/test_models/AbsComponentModelWeight/Full/KerasCompWeightFullPipeline/KerasCompWeightFullPipeline.json"
			r := genLinearRegressor(types.AbsComponentModelWeight, "", initModelURL)
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
			_, err = r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Get Container Components Power By Linear Regression Estimator without ModelServerEndpoint", func() {
			// Estimate Container Components Absolute Power using Linear Regression
			initModelURL := "https://raw.githubusercontent.com/sustainable-computing-io/kepler-model-server/main/tests/test_models/DynComponentModelWeight/CgroupOnly/ScikitMixed/ScikitMixed.json"
			r := genLinearRegressor(types.DynComponentModelWeight, "", initModelURL)
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, containerFeatureValues := range containerFeatureValues {
				r.AddContainerFeatureValues(containerFeatureValues) // add samples to the power model
			}
			_, err = r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// TODO: right now we don't have a pre-trained power model for node platform power, we should create a test when it is available.
})
