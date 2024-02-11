function expandstuff() {
	var els = document.querySelectorAll(".honk details")
	for (var i = 0; i < els.length; i++) {
		els[i].open = true
	}
}

function updatedonker(el) {
	el = el.parentElement
	el.children[1].textContent = el.children[0].value.slice(-20)
}

(function() {
	var expand = document.querySelector("button.expand")
	if (expand) {
		expand.onclick = expandstuff
	}

	var donk = document.querySelector("#donker input")
	if (donk) {
		donk.onchange = function() {
			updatedonker(this);
		}
	}
})()
