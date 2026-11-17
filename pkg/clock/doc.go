// Package clock abstracts the wall clock so Drover services can be tested
// deterministically.
//
// Production code takes a Clock and is given RealClock{}. Tests give it a
// *MockClock and move time with Add or SetTime. MockClock is safe for
// concurrent use.
package clock
