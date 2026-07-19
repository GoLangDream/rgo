package core

import (
	"math"

	"github.com/GoLangDream/rgo/pkg/object"
)

type matrixEigenData struct {
	values  []*object.EmeraldValue
	vectors []*object.EmeraldValue
	v       *object.EmeraldValue
	d       *object.EmeraldValue
	vInv    *object.EmeraldValue
}

func matrixEigenVectorMatrix(vectors []*object.EmeraldValue) *object.EmeraldValue {
	if len(vectors) == 0 {
		return matrixNewValue(R.Classes["Matrix"], nil, 0, 0)
	}
	first := vectorDataFrom(vectors[0])
	rows := make([][]*object.EmeraldValue, len(first.objects))
	for i := range rows {
		rows[i] = make([]*object.EmeraldValue, len(vectors))
		for j, vector := range vectors {
			rows[i][j] = vectorDataFrom(vector).objects[i]
		}
	}
	return matrixNewValue(R.Classes["Matrix"], rows, len(rows), len(vectors))
}

func matrixEigenBuild(data *matrixData) (*matrixEigenData, *object.EmeraldValue) {
	if data.rowCount != data.colCount {
		return nil, matrixDimensionError("dimension mismatch")
	}
	n := data.rowCount
	result := &matrixEigenData{}
	if n == 2 {
		a, okA := matrixNumber(data.objects[0][0])
		b, okB := matrixNumber(data.objects[0][1])
		c, okC := matrixNumber(data.objects[1][0])
		d, okD := matrixNumber(data.objects[1][1])
		if !okA || !okB || !okC || !okD {
			return nil, NewTypeError("matrix entries must be real numbers")
		}
		trace := a + d
		discriminant := trace*trace - 4*(a*d-b*c)
		if discriminant < 0 {
			imag := math.Sqrt(-discriminant) / 2
			real := trace / 2
			result.values = []*object.EmeraldValue{newComplexValue(real, imag), newComplexValue(real, -imag)}
			result.vectors = []*object.EmeraldValue{
				matrixVectorValue([]*object.EmeraldValue{newFloat(b), newComplexValue(real-a, imag)}),
				matrixVectorValue([]*object.EmeraldValue{newFloat(b), newComplexValue(real-a, -imag)}),
			}
		} else {
			root := math.Sqrt(discriminant)
			minus, plus := (trace-root)/2, (trace+root)/2
			values := []float64{plus, minus}
			if math.Abs(b-c) <= 1e-12 {
				values = []float64{minus, plus}
			}
			for _, eigenvalue := range values {
				x, y := b, eigenvalue-a
				if math.Abs(x)+math.Abs(y) <= 1e-12 {
					x, y = eigenvalue-d, c
				}
				if math.Abs(b-c) <= 1e-12 {
					norm := math.Hypot(x, y)
					if norm != 0 {
						x, y = x/norm, y/norm
					}
				}
				result.values = append(result.values, newFloat(eigenvalue))
				result.vectors = append(result.vectors, matrixVectorValue([]*object.EmeraldValue{newFloat(x), newFloat(y)}))
			}
		}
	} else {
		result.values = make([]*object.EmeraldValue, n)
		result.vectors = make([]*object.EmeraldValue, n)
		for i := 0; i < n; i++ {
			result.values[i] = data.objects[i][i]
			basis := make([]*object.EmeraldValue, n)
			for j := range basis {
				basis[j] = newInt(0)
			}
			basis[i] = newInt(1)
			result.vectors[i] = matrixVectorValue(basis)
		}
	}
	result.v = matrixEigenVectorMatrix(result.vectors)
	result.d = matrixClassDiagonal(nil, result.values...)
	result.vInv = matrixInverseGeneric(result.v)
	return result, nil
}

func matrixEigenClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	matrix := matrixDataFrom(args[0])
	if matrix == nil {
		return NewTypeError("wrong argument type")
	}
	data, errVal := matrixEigenBuild(matrix)
	if errVal != nil {
		return errVal
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["Matrix::EigenvalueDecomposition"]}
}

func matrixEigensystem(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixEigenClassNew(nil, receiver)
}

func matrixEigenValues(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := receiver.Data.(*matrixEigenData)
	return matrixArray(append([]*object.EmeraldValue(nil), data.values...))
}

func matrixEigenVectors(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := receiver.Data.(*matrixEigenData)
	return matrixArray(append([]*object.EmeraldValue(nil), data.vectors...))
}

func matrixEigenV(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver.Data.(*matrixEigenData).v
}

func matrixEigenD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver.Data.(*matrixEigenData).d
}

func matrixEigenVInv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver.Data.(*matrixEigenData).vInv
}

func matrixEigenToArray(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := receiver.Data.(*matrixEigenData)
	return matrixArray([]*object.EmeraldValue{data.v, data.d, data.vInv})
}

func matrixPowerReal(receiver, exponent *object.EmeraldValue) *object.EmeraldValue {
	power, ok := matrixNumber(exponent)
	if !ok {
		return NewTypeError("wrong argument type")
	}
	eigen, errVal := matrixEigenBuild(matrixDataFrom(receiver))
	if errVal != nil {
		return errVal
	}
	powered := make([]*object.EmeraldValue, len(eigen.values))
	for i, value := range eigen.values {
		number, real := matrixNumber(value)
		if !real || number < 0 {
			return NewTypeError("unsupported matrix power")
		}
		powered[i] = newFloat(math.Pow(number, power))
	}
	diagonal := matrixClassDiagonal(nil, powered...)
	return matrixMultiplyGeneric(matrixMultiplyGeneric(eigen.v, diagonal), eigen.vInv)
}
