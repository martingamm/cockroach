cockroach
=========

Simple concurrent web-crawler written in Go. Does not obey robots.txt
and is only meant as an educational exercise.

More information in comments.

The code is excessively commented on request since it was originally
written as a test for an interview.

### TODO
* If it can't reach the desired depth (hard to do on today's internet,
  but if the seed-URL is dead for example), it panics since all
  goroutines go to sleep and we get a deadlock.

* Shut down more gracefully.
