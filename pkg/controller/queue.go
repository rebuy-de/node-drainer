package controller

type Queue []Request

// Add inserts the specified element into this queue.
func (q *Queue) Add(r Request) {
	*q = append(*q, r)
}

// Poll retrieves, but does not remove, the head of this queue, or returns null
// if this queue is empty.
func (q *Queue) Poll() Request {
	if len(*q) < 1 {
		return nil
	}

	r := (*q)[0]
	*q = (*q)[1:]

	return r
}
