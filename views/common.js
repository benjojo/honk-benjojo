
let re_link = /https?:[^ ]*/
function hotkey(e) {
	if (e.ctrlKey || e.altKey)
		return
	if (e.code == "Escape") {
		var menu = document.getElementById("topmenu")
		menu.open = false
		return
	}
	if (e.target instanceof HTMLInputElement)
		return
	if (e.target instanceof HTMLTextAreaElement) {
		let k = e.key
		if (!k.match(/^[a-zA-Z]$/)) return
		let t = e.target
		let start = t.selectionStart
		let end = t.selectionEnd
		if (start == end)
			return
		let val = t.value.substr(start, end-start)
		if (val.match(re_link)) {
			let s = t.value.substr(0, start)
			let e = t.value.substr(end)
			let repl = `[](${val})`
			t.value = s + repl + e
			t.selectionStart = start+1
			t.selectionEnd = start+1
		}
		return
	}

	switch (e.code) {
	case "KeyR":
		refreshhonks(document.getElementById("honkrefresher"));
		break;
	case "KeyS":
		oldestnewest(document.getElementById("newerscroller"));
		break;
	case "KeyJ":
		scrollnexthonk();
		break;
	case "KeyK":
		scrollprevioushonk();
		break;
	case "KeyM":
		var menu = document.getElementById("topmenu")
		if (!menu.open) {
			menu.open = true
			menu.querySelector("a").focus()
		} else {
			menu.open = false
		}
		break
	case "Slash":
		document.getElementById("topmenu").open = true
		document.getElementById("searchbox").focus()
		e.preventDefault()
		break
	}
}

(function() {
	document.addEventListener("keydown", hotkey)
	var totop = document.querySelector(".nophone")
	if (totop) {
		totop.onclick = function() {
			window.scrollTo(0,0)
		}
	}
	var els = document.getElementsByClassName("donklink")
	while (els.length) {
		let el = els[0]
		el.onclick = function() {
			el.classList.remove("donk")
			el.onclick = null
			return false
		}
		el.classList.remove("donklink")
	}

})()
