package main

func subsets(nums []int) [][]int {
	var res [][]int
	res = append(res, []int{})
	for i, num := range nums {
		parentArray := []int{num}
		res = append(res, parentArray)
		findSubsets(parentArray, &nums, i+1, &res)
	}

	return res
}

func findSubsets(curArray []int, nums *[]int, left int, res *[][]int) {
	if left >= len(*nums) {
		return
	}

	for i := left; i <= len(*nums)-1; i++ {
		newParSet := make([]int, len(curArray))
		copy(newParSet, curArray)
		newParSet = append(newParSet, (*nums)[i])
		*res = append(*res, newParSet)
		findSubsets(newParSet, nums, i+1, res)
	}
}
